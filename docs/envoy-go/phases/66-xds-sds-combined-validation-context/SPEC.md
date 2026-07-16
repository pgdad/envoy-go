# SPEC 66 — xDS **SDS `combined_validation_context`** (the FOURTH xDS-family row; lift the `commonTLSContextToConfig` `combined_validation_context is not supported in phase 03` reject — downstream + live-provider + `require_client_certificate: true` ONLY; upstream + QUIC keep the BYTE-IDENTICAL substring, ADR-0080 — and honor a CVC whose trust anchor is the SDS-delivered `CertificateValidationContext` phase 65 landed, installed as the downstream mTLS `ClientCAs`)

> **Stage:** SPEC (lifecycle-state 1 → 2). Docs-only — ZERO production `.go`. Fresh worktree off master `b9f5a1c8`, branch `phase-66-spec`, worktree `.worktrees/phase-66-spec`, per `feedback_git_worktrees`.
>
> **Row 66 STAYS `in-progress`** — the IMPL flips it `done` at its six-gate (ADR-0106). ADR-0287's §Context drafts HERE (ADR-0044); §Decision/§Consequences land at the IMPL.
>
> **⚠️ THIS SPEC OVERTURNS THE BRAINSTORM'S CENTRAL IMPLEMENTATION PLAN — ON EVIDENCE, BY EXECUTION.** The BRAINSTORM's motto was *"quoting is not executing"*, and it applied that motto to `proto.Merge` while **citing** `FetchInitialValidationContext`. This SPEC ran the probes the BRAINSTORM ordered and then re-derived the seam the BRAINSTORM assumed. The result: **the planned `proto.Clone` + `proto.Merge` implementation is STRUCTURALLY IMPOSSIBLE** as written (§3.1), the row's **"sharpest risk" (D-COMBVC-HYBRID-DS) DISSOLVES** (§3.4), **D-COMBVC-CLONE DISSOLVES** (§3.4), **D-COMBVC-PUREINLINE is REFUTED** — the BRAINSTORM's "legal per the proto — verified" is false (§5.2) — and **TWO reject arms the BRAINSTORM never named are MANDATORY** (§6). The BRAINSTORM's *conclusion* (zero new symbols in `internal/xds`) SURVIVES, but **only via an equivalence theorem it did not state, and for reasons opposite to the ones it gave** (§3.2). **⇒ The PLAN must read the BRAINSTORM for the SUBJECT and the ENVELOPE only; its §6 implementation instructions do not compile.**
>
> **⚠️ AND THIS SPEC'S OWN FIRST DRAFT CARRIED FOUR DEFECTS — ONE SEVERE — ALL FOUND BY AN ADVERSARIAL PASS, ALL RECORDED IN §1.2 RATHER THAN HIDDEN.** The severe one (*"QUIC is the guard's only live consumer"*) was **false**, was inherited from a **false landed code comment** in the very doc region this SPEC audited for stale comments, and was **bound verbatim for DECISIONS.md**. Every one of the four landed in a section where this SPEC was **most confident it had already caught the error**. The equivalence theorem **survived** the attack and gained a premise (P5). **The lesson generalizes past documents: a landed CODE COMMENT is not evidence either, and a probe that RUNS but cannot DISCRIMINATE is no better than a citation.**
>
> **Baselines RE-VERIFIED against master tip `b9f5a1c8` this session (`git fetch` first; NOT copied from the router):** fixtures **110** (`ls -d test/fixtures/[0-9]*/ | wc -l`; numeric tail `0108-xds-sds-validation-context`) · fuzzers **55** (`grep -rn '^func Fuzz' --include='*.go' internal/ | wc -l`) · DECISIONS tail **ADR-0286** (next-free **ADR-0287**) · BackendKind tail **38** (`H2GoawayResponder`) · stat surface **1201** (docs-verified; no mechanical count exists) · go.mod modules **2** (the phase-61.2 lineage figure; the single `go.mod` requires **67** modules — re-counted this session).
>
> **Termination sentinel RE-RUN MECHANICALLY this session — it does NOT fire; `stop` NOT created.** (1) prints `NOT DONE: row 66`; (2) prints THREE live `candidates:` sentences (HTTP/3 `:176`, xDS `:186`, Observability `:196`); (3) prints `NEVER OPENED: gRPC`, `Runtime`, `WASM`.

---

## 1. Purpose / Mission

Lift the `combined_validation_context` (CVC) reject in `commonTLSContextToConfig` for **downstream + live-provider + `require_client_certificate: true` ONLY**, and honor the knob: a listener declares an inline `default_validation_context` alongside a `validation_context_sds_secret_config`, and envoy-go installs the resulting trust anchor as `cfg.ClientCAs` + `ClientAuth = RequireAndVerifyClientCert`.

The served `tls.v3.Secret` wire is **byte-identical to `0108`'s** — same type URL, same `validation_context` oneof arm. The management server cannot tell a CVC client from a plain-SDS client. **CVC is a LISTENER-CONFIG shape, not a SERVED-SECRET shape.**

### 1.1 BRAINSTORM anticipations — what HELD, what was REFUTED (ADR-0044)

**HELD (re-derived from source this session, cited by SYMBOL):**
- The CVC reject arm, its exact substring, and the phase-65 gate shape (§6, §12).
- The four inline sub-field rejects are **BYPASSED** under CVC — the generated getter makes `GetValidationContext()` return nil (§3.5). **CONFIRMED against the generated code, not assumed.**
- The boot pre-scan asserts on ONE type only and returns a **silent `(nil, nil)`** at `seen == 0` (§3.6).
- **D-COMBVC-UPSTREAM-DEAD — CONFIRMED** by adversarial re-derivation (§3.7). The upstream CVC arm is unreachable. *(But this SPEC's accompanying "QUIC is the guard's only live consumer" claim was **FALSE** — see R9.)*
- `loadTrustedCAPool` reads **only** `vc.GetTrustedCa()` (§3.2).
- `proto.Merge` **does** recursively merge `trusted_ca`, **does** produce a hybrid `DataSource`, and **does** mutate `dst` — all three OBSERVED (§3.3). The BRAINSTORM's corrected proto quote, including the trailing singular sentence, is **accurate** (verified to the end of the comment block).
- Stat surface **+0**; fuzzers **+0**; go.mod **+0**; ZERO new packages; the 60.2 cycle guard stands trivially (§3.8, §7, §8).

**REFUTED / RESHAPED — each by execution, each recorded in full below:**

| # | BRAINSTORM claim | Verdict | § |
|---|---|---|---|
| R1 | "`FetchInitialValidationContext` (verbatim) + `proto.Clone` the default + `proto.Merge` the dynamic onto the clone" | **IMPOSSIBLE** — the provider returns `*x509.CertPool`; the proto message is destroyed inside `internal/xds` and never escapes it. You cannot `proto.Merge` a `CertPool`. | §3.1 |
| R2 | D-COMBVC-CLONE — "a `proto.Clone` guard is MANDATORY" | **DISSOLVED** — the adopted design calls neither `Merge` nor `Clone`, so the bootstrap is never mutated. | §3.4 |
| R3 | D-COMBVC-HYBRID-DS — "the row's sharpest genuine risk" | **DISSOLVED** — the hybrid `DataSource` is never constructed under the adopted design. Also INCONCLUSIVE on the reference side even if it had been. | §3.4 |
| R4 | D-COMBVC-PUREINLINE — a CVC with no SDS half is "legal per the proto (a plain, optional message field — **verified**)" | **REFUTED** — PGV marks **BOTH** halves `value is required`; the live reference rejects it at config-validate. The BRAINSTORM's "verified" was false. | §5.2 |
| R5 | D-COMBVC-EMPTY-DYNAMIC — "pin whether that is the reference's behavior" | **ANSWERED, and it is a REAL divergence** — the reference ACKs an empty dynamic context, boots, and **serves traffic against the DEFAULT CA**; envoy-go boot-fails. | §3.9 |
| R6 | D-COMBVC-REQUIRE-FALSE — "does the reference honor a CVC anchor when require is false?" | **ANSWERED — YES**, verify-if-presented confirmed live. envoy-go silently ignores ⇒ a REAL divergence, scoped out as a NAMED BOUNDARY. | §3.10 |
| R7 | §2.6 "the arm must nil-check the *inner* field … otherwise a pure-inline CVC trips `ParseSDSConfig`" | **RE-BASED** — the hazard framing came from R4's false premise. The nil-check survives as an explicit **reject**, not as defensive plumbing. | §3.6 |
| R8 | The §6 roster is complete | **INCOMPLETE** — two mandatory reject arms missing (§6), plus three STALE/FALSE code comments the row must fix (§11). | §6, §11 |

### 1.2 ⚠️ FOUR defects in THIS SPEC's OWN first draft — found by an adversarial pass, recorded not hidden

The router required an adversarial verification pass (it found eleven defects in the BRAINSTORM's first draft). **It found four here — one SEVERE — and every one of them landed in a section where this SPEC was most confident it had already caught the error.** Recorded in place rather than silently corrected, because the pattern is the deliverable:

| # | Sev | Defect | Fate |
|---|---|---|---|
| **R9** | **SEVERE** | *"QUIC is the CVC guard's ONLY live consumer"* — **FALSE.** `validate.Bootstrap` passes a **nil** provider too, so `side=="downstream" && provider==nil` is reachable on an **ordinary TCP listener**. Proven by **executing** `validate.BootstrapFile`. **Inherited from a FALSE landed code comment** — in the very doc region this SPEC audited and caught two *other* stale comments in. Was **bound for DECISIONS.md**. | §3.7 rewritten; §6/§9/§11/§14 propagated |
| R10 | moderate | *"a CVC listener silently boots with no trust anchor"* — a **false control-flow claim, self-contradictory in its own sentence**: the nil-provider gate produces a **loud boot-FAIL**, not a silent boot. Conclusion (co-edit mandatory) survives; mechanism was wrong. Also bound for DECISIONS.md. | §3.6 corrected |
| R11 | moderate | The §3.3 row-8 `BoolValue` probe was **VACUOUS** — one input (`true⊕false→true`) that **cannot discriminate** (`OR(T,F)=T` too), carrying a negative conclusion. All four combos: **extensionally OR**. | §3.3 corrected |
| R12 | moderate | §5.3's roster claimed *"re-enumerated in full from the generated type"* while carrying **8 of 15** fields — the exact accusation it levels at the BRAINSTORM, one clause later. | §5.3 re-enumerated **by reflection** |

**The theorem (§3.2) SURVIVED the attack** — no counterexample exists on its stated domain — and the pass contributed **P5**, a load-bearing premise the draft had not noticed.

**The generalizations for the PLAN — three, each earned:**
1. **Execute the semantics AND re-derive the seam's signature.** The BRAINSTORM told its successor to *run* `proto.Merge`, and was right — but its residual defect was **citing the seam it planned to build on**. Running `proto.Merge` was necessary and insufficient; the decisive fact was one read of `SecretProvider`'s **return type**. A correct motto, applied to only one of the two things it needed to cover.
2. **In-codebase prose is not evidence either** (R9). The project knows a brief's, a SPEC's, a PLAN's and a BRAINSTORM's citations are not evidence. **A landed code comment is not evidence.** It is uncited prose with the authority of proximity to the code — and when the file **disagrees with itself** (§3.7), proximity picks the wrong sentence.
3. **A probe must DISCRIMINATE, not merely run** (R11). One data point consistent with both hypotheses, carrying a negative conclusion, is the same defect as a citation — `reference_vacuous_break_receiver_normalizes` inside an empirical pin block.

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.2 + §8, as CORRECTED here)

Not upstream CVC (structurally dead — §3.7). Not QUIC CVC (nil provider ⇒ the reject stays, ADR-0080). **Not `require_client_certificate: false`** (§3.10 — a NAMED BOUNDARY, with the reference behavior now PINNED by probe rather than assumed). Not the compose-two edge (`seen == 1` — §3.6). Not SDS rotation (§3.11 records a newly-observed reference behavior that belongs to it). Not `crl` (a SHARED gap). Not the repeated-concatenate or bool-OR merge rules (structurally unreachable — §5.3). **Not exact `MergeFrom` fidelity in the empty-dynamic case** (§3.9 — the row's ONE named departure).

---

## 3. The change

### 3.1 ⚠️ The BRAINSTORM's implementation plan is STRUCTURALLY IMPOSSIBLE *(R1 — the load-bearing finding)*

RE-DERIVED from source. `internal/xds/provider.go`, the `SecretProvider` interface:

```go
type SecretProvider interface {
	FetchInitialCertificate(ctx context.Context, secretName string) (*stdtls.Certificate, error)
	FetchInitialValidationContext(ctx context.Context, secretName string) (*x509.CertPool, error)
}
```

`parseValidationSecret` (`internal/xds/secret.go`, symbol) unmarshals the `tls.v3.Secret`, applies its four sub-field rejects, calls `dataSourceBytes(vc.GetTrustedCa(), baseDir)`, builds an `*x509.CertPool`, and **returns the pool**. Its signature is `(*anypb.Any, string, string) (*x509.CertPool, error)`.

**The `*tlsv3.CertificateValidationContext` message is consumed and discarded inside `internal/xds`. It never crosses the seam.** `proto.Merge` operates on messages. Therefore:

> **The BRAINSTORM's §6 edit-site 3 — "`FetchInitialValidationContext` (verbatim) + `proto.Clone` the default + `proto.Merge` the dynamic onto the clone + `loadTrustedCAPool`" — cannot be written.** There is no message to clone, and nothing to merge onto it.

This is not a detail. It is the row's whole apply-point, and the BRAINSTORM's defining property ("ZERO new symbols in `internal/xds`") was asserted **on top of** it.

### 3.2 The fork — and the EQUIVALENCE THEOREM that resolves it *(D-COMBVC-SEAM — DECIDED: Design A)*

Three designs are available. Two are refuted by evidence; the third is adopted.

**Design B — a message-returning seam.** Add e.g. `FetchInitialValidationContextMessage(...) (*tlsv3.CertificateValidationContext, error)` to `internal/xds`, merge in `internal/tls`, then build the pool there. Buys exact `MergeFrom` fidelity. **Costs:** +1–2 new symbols in `internal/xds` (breaking the row's headline property), and it forces `parseValidationSecret`'s four sub-field rejects and its pool-building to either move or duplicate into `internal/tls` — real surgery on a landed, passing chain. **REJECTED as the row's default; recorded as the lift path if §3.9's boundary is later closed.**

**Design C — union the CA pools.** Merge at the `*x509.CertPool` level (default pool ∪ SDS pool). Needs no new symbol. **REFUTED BY LIVE PROBE** (§3.3, arm (d)): the reference, given default `trusted_ca={filename: CA_Y}` + dynamic `{inline_bytes: CA_X}`, **accepts client_X and REJECTS client_Y with `UNKNOWN_CA`**. A pool union would accept **both**. Design C is provably wrong cross-side — and it is exactly the "union two" behavior the proto's own trailing sentence (*"**The resulting** `CertificateValidationContext`"*, singular) forecloses. **REJECTED.**

**Design A — the SDS-delivered pool wins; `default_validation_context.trusted_ca` is not read. ADOPTED.** No `proto.Merge`. No `proto.Clone`. No new symbol in `internal/xds`. `FetchInitialValidationContext` is reused **verbatim** — and this time the word is earned.

**THE EQUIVALENCE THEOREM (the SPEC's central contribution; each premise OBSERVED or RE-DERIVED, none cited):**

> **Claim.** On envoy-go's honored surface, Design A is *observationally identical* to a true `proto.Merge` of the dynamic context onto the default — for **every** input on which the SDS fetch succeeds.
>
> **P1.** envoy-go honors exactly ONE field of `CertificateValidationContext`: `trusted_ca`. *(`loadTrustedCAPool` touches `vc` once — `loadDataSource(vc.GetTrustedCa(), baseDir)` — then `x509.NewCertPool()`/`AppendCertsFromPEM`. RE-DERIVED from source.)* So the merged message's only observable is the **bytes** its `trusted_ca` resolves to.
>
> **P2.** `trusted_ca` is a `*core.v3.DataSource` **message**, so `proto.Merge` recurses into it; its `specifier` **oneof** REPLACES when set in src. *(OBSERVED, §3.3 PROBE 1: `{filename:"/ca_y.pem", watched_directory:{path:"/watch"}} ⊕ {inline_bytes:"CA_X"} ⇒ {inline_bytes:"CA_X", watched_directory:{path:"/watch"}}`.)*
>
> **P3.** A successful SDS fetch **guarantees** the dynamic `trusted_ca`'s specifier is SET. *(`parseValidationSecret` → `dataSourceBytes(vc.GetTrustedCa(), …)`; `dataSourceBytes` switches on `ds.GetSpecifier()` and its `default:` arm ERRORS — `"none of inline_bytes, inline_string, filename set"` — for a nil `trusted_ca` AND for a non-nil `DataSource` whose specifier is unset. A nil receiver is getter-safe, so both shapes reach `default:`. RE-DERIVED from source.)*
>
> **P4.** Therefore, on any successful fetch, P2+P3 ⇒ the merged `trusted_ca`'s specifier is **always the dynamic's**, and P1 ⇒ the default's `trusted_ca` contributes **nothing observable**. The only surviving default-half sub-field is `watched_directory`, which P1 makes **inert** (`loadTrustedCAPool` never reads it).
>
> **P5 — the premise the theorem RESTS ON and the first draft did not notice** *(found by the adversarial pass; recorded because it is a **coincidence**, not a guarantee)*. A true merge would resolve the merged `trusted_ca{filename: <relative>}` against **`internal/tls`'s** `baseDir`; Design A resolves it against **`internal/xds`'s**. Two different variables — **so the theorem would be FALSE if they ever differed.** They do not: `main.go` passes `filepath.Dir(*cfgPath)` to **both** `NewSDSProvider` and `boot.Construct` (RE-DERIVED — same expression, same value). Relatedly, `internal/xds`'s `dataSourceBytes` and `internal/tls`'s `loadDataSource` are **arm-for-arm identical** (same four cases; `environment_variable` errors in both; `default:` errors in both) — a deliberate duplication forced by the 60.2 cycle guard. **⚠️ If a future row lets those two `baseDir` values diverge, or lets the two DataSource readers drift apart, it SILENTLY FALSIFIES this theorem and the CVC path goes wrong with no test to catch it.** The PLAN should carry a comment at the apply-point naming P5, and §13 carries it as a standing hazard.
>
> **∎ The merged pool ≡ the SDS pool, exactly.** Design A computes that pool directly and skips the merge. □

**The theorem was attacked adversarially and SURVIVED** — an independent pass told to default to REFUTED hunted counterexamples across the degraded edges (`inline_bytes: []` and `inline_string: ""` reach `AppendCertsFromPEM` and fail ⇒ *outside* the domain; `filename: ""` ⇒ EISDIR ⇒ outside; a `default_validation_context` carrying **no** `trusted_ca` ⇒ merge and Design A agree on the dynamic's), probed the suspected `dataSourceBytes`/`loadDataSource` asymmetry (**none exists**), and surfaced P5. **No counterexample exists on the stated domain.**

**The theorem's scope is its honesty.** It holds *whenever the fetch succeeds*. It says nothing about the case where the fetch **fails** — which is precisely §3.9's named departure, the theorem's exact complement. Design A is not "close enough"; it is **provably exact on its stated domain**, with the domain's boundary named rather than blurred.

**Why this matters beyond phase 66:** the BRAINSTORM reached the right headline (zero new xds symbols) through reasoning that was wrong at every step — it thought it would merge, and merging is what would have *forced* a new symbol. The conclusion survives because a **different, stronger** argument happens to hold. A PLAN that inherited the BRAINSTORM's reasoning would have written unbuildable code.

### 3.3 SPEC-time empirical pin block — `proto.Merge` / `proto.Clone` EXECUTED *(D-COMBVC-MERGERULES / -CLONE — the router's obligation #1)*

Run in-session against **real** `tlsv3.CertificateValidationContext` / `corev3.DataSource` messages at the repo's pinned `google.golang.org/protobuf v1.36.11` + `go-control-plane/envoy v1.32.4`. Throwaway probe; not committed. **Observed output, verbatim:**

| # | Input | OBSERVED | Pins |
|---|---|---|---|
| 1 | `{filename:"/ca_y.pem", watched_directory:{path:"/watch"}}` ⊕ `{inline_bytes:"CA_X"}` | `{inline_bytes:"CA_X", watched_directory:{path:"/watch"}}` | recursive message merge; specifier oneof REPLACES; **`watched_directory` SURVIVES ⇒ hybrid DataSource** |
| 2 | `Merge(victim, dyn)` with no clone | `victim` before `{filename:"/ca_y.pem"}` → after `{inline_bytes:"CA_X"}` | **`Merge` MUTATES `dst`** |
| 3 | `Merge(Clone(boot), dyn)` | `boot` byte-identical before/after | the clone guard works — **moot under Design A** |
| 4 | `Merge(default, dynamic)` vs `Merge(dynamic, default)` | `{inline_bytes:"CA_X",…}` vs `{filename:"/ca_y.pem",…}` | **NOT commutative**; dynamic must be src |
| 5 | `default` ⊕ `{}` | `{filename:"/ca_y.pem", watched_directory:{path:"/watch"}}` | empty dynamic ⇒ **default INTACT** (→ §3.9) |
| 6 | `{hash:[D1,D2]}` ⊕ `{hash:[S1]}` | `[D1 D2 S1]` | repeated-CONCATENATE rule **HOLDS** |
| 7 | `allow_expired_certificate` (plain bool), all four (d,s) | `TT→T`, `TF→T`, `FT→T`, `FF→F` | matches logical-OR in **all four** |
| 8 | `require_signed_certificate_timestamp` (`*wrapperspb.BoolValue`), all four (d,s) | `TT→T`, `TF→T`, `FT→T`, `FF→F` | merged by **recursion** (it is a message), **but extensionally OR in all four** — the mechanism differs, the observable does **not**. Immaterial to this row. |

**⚠️ A mis-narration this SPEC caught in its OWN first probe pass — recorded because it is the phase's exact failure mode.** Pass 1 printed *"an UNPOPULATED proto3 scalar in src does NOT override"* under a `trusted_ca:{filename:""}` case whose result was `""`. **That conclusion line was wrong, and the probe output already contradicted it** — the result `""` proves the src **DID** override. A oneof set to a zero value is **populated** (presence is carried by the oneof case, not the value). Pass 2 re-probed and separated the two presence rules:

| Shape | OBSERVED merged `trusted_ca` | Rule |
|---|---|---|
| dynamic `{}` (no `trusted_ca`) | `{filename:"/ca_y.pem", watched_directory:{path:"/watch"}}` | default INTACT |
| dynamic `{trusted_ca:{}}` (message present, **oneof UNSET**) | `{filename:"/ca_y.pem", watched_directory:{path:"/watch"}}` | default INTACT — **identical to the above** |
| dynamic `{trusted_ca:{filename:""}}` (**oneof SET to zero**) | `{filename:"", watched_directory:{path:"/watch"}}` | **default CLOBBERED to empty** |
| plain scalar contrast: `WatchedDirectory{path:"/watch"}` ⊕ `{path:""}` | `{path:"/watch"}` | plain zero scalar does **NOT** override |

**Two different presence rules operate inside one merge** — oneof-presence (a zero value overrides) and proto3 scalar-presence (a zero value does not). The BRAINSTORM's D-COMBVC-EMPTY-DYNAMIC conflated all three degraded shapes into one; they are **three**, and they do not agree.

**And the proto's "Boolean Fields: logical OR" bullet is accurate but for a reason it does not state** (row 7): proto3 plain-bool merge is "copy src iff non-zero", which is *extensionally* logical-OR. It is emergent from presence semantics, not a special rule. Immaterial to this row (§5.3) but recorded so a future roster row does not mis-generalize the bullet.

**⚠️ A VACUOUS PROBE this SPEC shipped in its first draft, inside this very pin block — caught by the adversarial pass.** The draft probed row 8 with the **single** input `Bool(true) ⊕ Bool(false) → true` and concluded *"bool-OR does not govern it."* **That input cannot discriminate**, because `OR(true,false) = true` as well: the observation is consistent with both hypotheses, and a negative conclusion was drawn from it anyway. Re-run across **all four** combinations, `BoolValue` is **extensionally logical-OR in every one** — byte-for-byte the plain-bool pattern (`proto.Merge` recurses into the wrapper, whose `value` member is itself a plain proto3 bool obeying "copy iff non-zero"). The wrapper differs from a plain bool only in that UNSET is *representable*, **not** in its merge observable. **This is `reference_vacuous_break_receiver_normalizes` occurring inside the SPEC's own empirical pin block — in the row whose stated purpose was "so a future roster row does not mis-generalize the bullet." It mis-generalized the bullet.** Recorded, not silently corrected: *running* a probe is not enough — the input must be able to **distinguish the hypotheses**, and one data point carrying a negative conclusion is the same defect as a citation.

### 3.4 D-COMBVC-HYBRID-DS and D-COMBVC-CLONE — **BOTH DISSOLVE** *(R2, R3)*

The BRAINSTORM ranked D-COMBVC-HYBRID-DS as **"the row's sharpest genuine risk"** and D-COMBVC-CLONE as **"MANDATORY, not optional."** Under Design A **neither exists**:

- **No `proto.Merge` call ⇒ no hybrid `DataSource` is ever constructed.** The hybrid was an artifact of the merge; deleting the merge deletes the artifact. The `watched_directory` leak has no site to occur at.
- **No `proto.Merge` call ⇒ `dst` is never mutated ⇒ no `proto.Clone` guard is needed.** The parsed bootstrap is untouched because nothing writes to it. The BRAINSTORM's "the row owes a small hand-written wrapper, not zero code" is **void**.

**The reference-side probe is recorded for completeness and is INCONCLUSIVE — honestly so.** A live probe (fresh container per arm, `envoyproxy/envoy:contrib-v1.37.2`) found the hybrid boots clean, **SDS-ACKs**, and validates against CA_X — with the watch path present, absent, **and nonexistent** (`/nonexistent_watch_zzz`: no error, no NACK). But the prober **could not make `watched_directory` produce an observable effect in ANY arm — including the unambiguous case where it arrives via the DYNAMIC half.** With no positive control, "the reference ignores `watched_directory`" is **NOT established**; only "no observable difference" is. *(That distinction is the phase's motto applied against the prober's own convenience, and it is recorded as INCONCLUSIVE rather than upgraded.)* **Design A makes the question moot regardless** — which is why the ADOPTED design is also the one that retires the row's largest unknown.

### 3.5 The four inline sub-field rejects are BYPASSED under CVC — the row's real correctness obligation *(D-COMBVC-REJECT-REPOINT — CONFIRMED)*

CONFIRMED against the **generated getter** (`go-control-plane/envoy@v1.32.4`, `extensions/transport_sockets/tls/v3/tls.pb.go`):

```go
func (x *CommonTlsContext) GetValidationContext() *CertificateValidationContext {
	if x, ok := x.GetValidationContextType().(*CommonTlsContext_ValidationContext); ok {
		return x.ValidationContext
	}
	return nil
}
```

Under a CVC the oneof holds `*CommonTlsContext_CombinedValidationContext` ⇒ the type-assert fails ⇒ **nil** ⇒ `commonTLSContextToConfig`'s `if vc := c.GetValidationContext(); vc != nil { … }` block is **skipped entirely**, taking all four rejects (`custom_validator_config`, `match_typed_subject_alt_names`, `verify_certificate_hash`, `verify_certificate_spki`) with it.

**Lifting the CVC envelope without re-pointing that block at `cvc.GetDefaultValidationContext()` would SILENTLY ACCEPT all four on the default half.** This is `reference_strict_reject_sibling_typeurl_gap` in its exact shape: **lifting an envelope is not license to silently accept sub-fields envoy-go cannot honor.** All four are unproven under CVC today ⇒ **each needs its OWN test** (`reference_fatalf_makes_assertions_unreachable`: `Errorf` per property).

The served half keeps its own four rejects in `parseValidationSecret` — **untouched**, and now covering the dynamic half while the re-pointed block covers the default half. The two rosters are deliberately identical.

### 3.6 The boot pre-scan third arm *(D-COMBVC-PRESCAN / -SINGLESLOT — CONFIRMED, re-based)*

RE-DERIVED: `NewSDSProvider` (`internal/boot/boot.go`) type-asserts on **exactly one** type — `*tlsv3.CommonTlsContext_ValidationContextSdsSecretConfig`. No CVC assert exists. A CVC listener ⇒ `seen == 0` ⇒ **`return nil, nil`** — a **silent nil, not an error** — ⇒ the tls gate's `provider == nil` half fires. **Lifting only the `config.go` reject is therefore INSUFFICIENT; `boot.go` is a MANDATORY co-edit.**

**⚠️ The FAILURE MODE, stated correctly — this SPEC's first draft got it wrong** (caught by the adversarial pass, and corrected here because §14's wording lands **verbatim in DECISIONS.md**). The draft said *"or a CVC listener silently boots with no trust anchor"* — **a false control-flow claim, and self-contradictory on its own sentence.** If the `provider == nil` half fires, `commonTLSContextToConfig` returns an **error**, which `NewDownstreamConfig` propagates *before* its `require_client_certificate` block is ever reached ⇒ **boot FAILS, loudly.** It cannot simultaneously "silently boot". **The correct statement: without the third arm, a CVC listener BOOT-FAILS with the retained `combined_validation_context is not supported in phase 03` reject — a MISLEADING message (the feature is supported; the pre-scan just never built a provider), not a silent trust-anchor bypass.** The **conclusion is unchanged** — the co-edit is mandatory, because without it the feature cannot work at all. Only the mechanism was wrong. *(Recorded rather than quietly fixed: this is exactly `feedback_brief_citations_not_evidence`'s uncited-PROSE shape, and it was heading for an ADR.)*

The third arm must extract `cvc.GetValidationContextSdsSecretConfig()`. **R7 re-bases the BRAINSTORM's framing:** it called the inner nil-check "a subtlety with no phase-65 analogue" *because* it believed a pure-inline CVC was legal (R4 — false). It is not legal (§5.2), so the correct disposition is **an explicit reject** (§6, `E2`), not a defensive nil-check that quietly tolerates the shape. The guard stays; its **meaning** changes from "tolerate" to "refuse".

`seen++` fires once ⇒ **`seen == 1`** ⇒ the `seen > 1` guard (`"multiple SDS-bound downstream TLS contexts unsupported (MVP takes one)"`) is **untouched** and the deferred compose-two edge **stays shut**. **D-COMBVC-SINGLESLOT CONFIRMED.**

*(Recorded, not fixed here: the landed plain-VC arm passes `vsc.ValidationContextSdsSecretConfig` with **no nil-check**. Traced — it cannot panic: `ParseSDSConfig` takes `configs[0]`, and `sc.GetName()` is nil-receiver-safe, yielding `"xds: sds: SdsSecretConfig name is required"`. A misleading message, not a crash. **Out of scope**; noted so the PLAN does not "fix" it and widen the row.)*

### 3.7 D-COMBVC-UPSTREAM-DEAD — CONFIRMED. **But "QUIC is the only live consumer" is FALSE — and this SPEC shipped it before an adversarial pass caught it.**

**The `upstream` half — CONFIRMED.** `NewUpstreamConfig` refuses **before** it ever calls `commonTLSContextToConfig`:

```go
vc := common.GetValidationContext()
if vc == nil || vc.GetTrustedCa() == nil {
    return nil, fmt.Errorf("tls: upstream: validation_context.trusted_ca is required (phase 03 does not permit unvalidated upstream TLS)")
}
```

Selecting the CVC oneof arm ⇒ `GetValidationContext()` nil (§3.5's getter) ⇒ refusal **here**, with a different message. So `tls: upstream: combined_validation_context is not supported in phase 03` is **unreachable**. **D-COMBVC-UPSTREAM-DEAD holds.**

**⚠️ THE `provider == nil` HALF HAS *TWO* LIVE CONSUMERS, NOT ONE. `validate.Bootstrap` is the second.** RE-DERIVED and then **EXECUTED**:

```
validate/validate.go:49:  _, err = boot.Construct(bs, cm, baseDir, allowH2C, nil, dm, httpClient, tracingProvider, nil)
                                                                                                              ^^^^ sdsProvider
internal/listener/manager.go:472 / :554:  internaltls.NewDownstreamConfig(ts, baseDir, sdsProvider)
internal/tls/config.go (NewDownstreamConfig):  commonTLSContextToConfig(..., "downstream", provider)
```

Driving `validate.BootstrapFile` on a **plain TCP (non-QUIC) listener** carrying each arm produces, OBSERVED:

```
### phase65 SDS-VC arm
  validate.BootstrapFile -> listener: "l0": filter_chains[0]: tls: downstream:
      SDS-bound validation_context_sds_secret_config is not supported in phase 03
### phase66 CVC arm
  validate.BootstrapFile -> listener: "l0": filter_chains[0]: tls: downstream:
      combined_validation_context is not supported in phase 03
```

**`side == "downstream" && provider == nil` is reachable on an ORDINARY TCP LISTENER.** QUIC is *a* live consumer; it is not the *only* one.

**⚠️ HOW THIS SPEC GOT IT WRONG — the lesson, recorded against itself.** The error was a **category confusion**: it enumerated the three direct callers of `commonTLSContextToConfig` (correct — two `"downstream"`, one `"upstream"`) and concluded the *guard* had one live consumer. But the `provider == nil` half is reached through callers of **`NewDownstreamConfig`**, one layer up, and `validate.Bootstrap` passes nil there. **The root cause is worse than the slip:** the claim was inherited from the LANDED CODE COMMENT at `commonTLSContextToConfig` — *"The guard below has exactly ONE live consumer: NewQUICDownstreamConfig"* — **which is itself FALSE**. This SPEC audited that exact doc region, caught two stale comments in it (§11 items 6–7), **swallowed the third, and promoted it to a heading marked "RE-DERIVED and CONFIRMED."** That is `feedback_brief_citations_not_evidence` in its purest form — *in-codebase prose treated as evidence* — and it is the phase-56.1 "OnDestroy fires twice" shape exactly: **a false control-flow claim that reached a SPEC and an ADR draft.** It was caught only by an adversarial pass told to default to REFUTED. **A third comment in the same file gets it right** (*"nil for upstream/**validate** callers, which never fetch SDS"*) and **contradicts** the one this SPEC trusted — the file disagrees with itself, and the SPEC picked the wrong sentence.

**⇒ Consequences the PLAN MUST absorb:**
1. After the lift, the retained CVC substring has **TWO** live consumers: `NewQUICDownstreamConfig` **and** `validate.Bootstrap`/`BootstrapFile`. **The gate must keep the `provider != nil` conjunct** or *both* leak in — QUIC silently, and `validate` into a false ACCEPT.
2. **A cross-side divergence this row creates and must name (§9 item 7):** the reference's `--mode validate` **ACCEPTS** a well-formed CVC config (§5.2's probe shows it rejects only the PGV-invalid shapes), while envoy-go's `validate.BootstrapFile` **REJECTS** it post-lift with a now-actively-misleading *"not supported in phase 03"* — the feature **is** supported; the validate path just has no provider. Pre-existing for phase 65's SDS-VC arm, but **this row is chartered to lift CVC and would leave its own validate path diverging.**
3. The false comment joins the §11 stale-comment fix list as **item 6b**.
4. **`validate/` is a package `internal/` greps do not reach** (§5.2's minor scope note is the same blind spot). §11's roster now names it.

*(Test-plan luck, recorded honestly: §10 already lists `provider == nil` as a negative arm **separately** from QUIC, so the required coverage survives the correction unchanged — by accident, not by design.)*

*(Separately: QUIC never evaluates `require_client_certificate` at all, so QUIC mTLS is absent regardless — an ambient boundary, not this row's.)*

### 3.8 The 60.2 cycle guard STANDS — trivially

`go list -deps ./internal/xds` (no `...`, per `reference_xds_config_seam_transitive_cycle_guard`) re-run this session: the in-repo dep set is exactly **`internal/stats` + `internal/xds`**. `internal/xds` is UNTOUCHED under Design A, so **no new edge is introduced anywhere**. `internal/boot` already imports both `internal/xds` and `internal/tls`.

*(Design B would also be import-safe — `internal/xds` already imports `tlsv3` — so the guard is **not** what rejects it; §3.2's cost argument is. Recorded so a future roller does not mistake a passing `go list` for a licence. The memory's own caveat holds: passing is necessary, not sufficient.)*

### 3.9 ⚠️ THE ROW'S ONE NAMED DEPARTURE — the empty-dynamic fallback *(D-COMBVC-EMPTY-DYNAMIC — ANSWERED BY LIVE PROBE; a REAL, non-vacuous divergence)*

The exact complement of §3.2's theorem: what happens when the fetch does **not** succeed. **PROBED LIVE** (`envoyproxy/envoy:contrib-v1.37.2`, fresh container **and fresh SDS server on a fresh port** per arm; three independent CAs so the server leaf is independent of the client verdict; a control arm reproduced the §3.3 specifier-REPLACE result to validate the harness).

Default `trusted_ca = {filename: CA_Y}`; the SDS server serves each degraded dynamic shape:

| Shape | Reference — OBSERVED | envoy-go — RE-DERIVED | Agree? |
|---|---|---|---|
| **(a)** dynamic `validation_context: {}` | **clean ACK** (`errorDetail=<nil>`), boots, **serves traffic**: `client_y` (DEFAULT CA_Y) **ACCEPTED**; `client_x` REJECTED | `dataSourceBytes` → `default:` arm ERRORS ⇒ fetch fails ⇒ **boot-FAIL** (ADR-0280) | **NO — a REAL divergence** |
| **(b)** `trusted_ca: {}` (specifier unset) | **NACK** — PGV: `CertificateValidationContextValidationError.TrustedCa … field: "specifier", reason: is required`; process stays up, CVC never initializes, **both** clients `REJECTED` | same `default:` arm ⇒ **boot-FAIL** | traffic: **yes**; lifecycle: **no** |
| **(c)** `trusted_ca: {filename: ""}` | **NACK** — PGV: `DataSourceValidationError.Filename: value length must be at least 1 characters` | `filepath.Join(baseDir,"")` → `os.ReadFile(<dir>)` ERRORS ⇒ **boot-FAIL** | traffic: **yes**; lifecycle: **no** |

**Three findings, none of which the BRAINSTORM had:**

1. **(a) is a REAL, live, traffic-serving divergence.** The reference **falls back to the default CA and serves**. envoy-go boot-fails. This is not a corner: "the SDS server returns a degraded/empty validation context" is an ordinary failure mode of a real management server.
2. **(c)'s predicted hazard NEVER MATERIALIZES on the reference.** §3.3 showed Go's `Merge` clobbers the default to `filename:""`; the reference **NACKs the secret via PGV before any merge happens**. The Go-side hazard is real; the reference-side hazard does not exist. *(Had this SPEC reasoned from `proto.Merge` alone — the BRAINSTORM's own instruction — it would have invented a divergence that the reference's validation layer forecloses. Executing the Go semantics was necessary; executing **the reference** was what kept the conclusion honest.)*
3. **The distinguishing mechanism is PGV on the served Secret** — which fires for (b)/(c) but **not** for (a), because an entirely empty `CertificateValidationContext` is a **valid** message. It ACKs, and merges as a no-op.

**DISPOSITION — ADOPT Design A and record (a) as a NAMED DEPARTURE, under the ADR-0280 boot-FAIL family.** Rationale: envoy-go's SDS is initial-fetch-only and ADR-0280 already establishes **boot-FAIL where the reference degrades gracefully** as this project's recorded posture; (a) is the same class, not a new one. Closing (a) requires Design B (§3.2) — a message seam plus relocating `parseValidationSecret`'s rejects — which is a larger row than this one is chartered to be. **The departure is NAMED, its mechanism is recorded, and §8 carries the lift path.**

**⚠️ The PLAN must NOT phrase the boundary as "envoy-go rejects where the reference rejects."** That is accurate for (b)/(c) **at the traffic level only**, and **false for (a)**. The honest phrasing is per-shape, and the fixture must not paper over it (§9).

### 3.10 D-COMBVC-REQUIRE-FALSE — disposed EXPLICITLY, with the reference PINNED *(R6; "a silence is not a scope decision")*

The router's obligation #3. The BRAINSTORM inherited this from phase-65 §8 and asked for an explicit disposition rather than a silence. **PROBED LIVE** — same config, one field changed:

| Client | `require=true` | `require=false` |
|---|---|---|
| `client_x` (trusted, SDS-served CA) | ACCEPTED | ACCEPTED |
| `client_y` (untrusted) | rejected `UNKNOWN_CA` | **rejected `UNKNOWN_CA`** |
| no client cert | rejected `CERTIFICATE_REQUIRED` | **ACCEPTED** |

**The reference HONORS the CVC trust anchor when `require_client_certificate` is false — textbook verify-if-presented.** envoy-go today: `NewDownstreamConfig`'s `if ctx.GetRequireClientCertificate().GetValue() {` block is skipped ⇒ no `ClientCAs`, `ClientAuth` stays zero (`NoClientCert`), **no SDS fetch happens at all**, and there is **no error**. **A real, PINNED divergence.**

**DISPOSITION — SCOPED OUT as a NAMED COVERAGE BOUNDARY. Not lifted.** Reasons: (i) phase 65 set the scope (mandatory mTLS) and carried the item; lifting it here would silently widen a row chartered as the smallest candidate; (ii) it is **not CVC-specific** — the identical silent-ignore applies to the plain SDS-VC path phase 65 landed and to the fully-inline path, so lifting it belongs in a row that fixes **all three** together, not one that fixes it only under CVC and leaves two siblings divergent; (iii) it needs `ClientAuth = VerifyClientCertIfGiven` plus a fetch-gate restructure (the fetch is currently *inside* the require block), which is an apply-point reshape, not a branch.

**This is a disposition, not a silence:** the shape is named, the reference behavior is **probed and pinned** rather than assumed, the mechanism is recorded, and §8 carries it with all three affected paths enumerated. **The BEHAVIOR_CONTRACT records it as a boundary at the IMPL (§10).**

### 3.11 A newly-observed reference behavior — recorded, out of scope

The probe found, unexpectedly: **the reference reloads an SDS-delivered `trusted_ca{filename:…}` on an atomic move with NO `watched_directory` configured at all** — an *implicit* watch on the secret file's own directory. In-place edit (same inode) does not trigger; a static (non-SDS) `validation_context` with `filename`+`watched_directory` does not reload either.

Orthogonal to CVC merging, and **inert in envoy-go** (SDS is initial-fetch-only — there is no reload path to diverge). **Recorded in §8 against the deferred SDS-rotation row**, whose scoping it materially informs: rotation is not only about honoring `watched_directory`.

---

## 4. Framework primitives — 0 new packages, 0 new go.mod modules

No new seam, no new discovery machine, no new applier, **no new symbol in `internal/xds`** (§3.2). `google.golang.org/protobuf` stays a direct require (and, under Design A, the row does not even call `proto.Merge`). Lineage figure stays **2 (+0)**.

**REUSES:** `FetchInitialValidationContext` / `parseValidationSecret` / the SotW stream — **verbatim, genuinely** · `loadTrustedCAPool` (phase 03) · `sdsserver.WithValidationContext` + `0108`'s driver.

---

## 5. Proto-field roster — RE-DERIVED @ `go-control-plane/envoy v1.32.4`

### 5.1 `CommonTlsContext_CombinedCertificateValidationContext` — four fields, TWO live

| Field | Status |
|---|---|
| `default_validation_context` | **LIVE** — and **PGV `value is required`** (§5.2) |
| `validation_context_sds_secret_config` | **LIVE** — and **PGV `value is required`** (§5.2) |
| `validation_context_certificate_provider` | proto-`[#not-implemented-hide:]` **AND** deprecated ⇒ out of scope by construction |
| `validation_context_certificate_provider_instance` | same ⇒ out of scope by construction |

### 5.2 ⚠️ **BOTH live halves are PGV-`required`** — D-COMBVC-PUREINLINE is **REFUTED** *(R4)*

The BRAINSTORM: *"a CVC with NO `validation_context_sds_secret_config` is legal per the proto (the field is a plain, optional message field — **verified**)."* **That is false.** RE-DERIVED from the generated validator (`tls.pb.validate.go`, symbol `CommonTlsContext_CombinedCertificateValidationContext.validate`):

```go
if m.GetDefaultValidationContext() == nil {
    err := …{field: "DefaultValidationContext", reason: "value is required"}
…
if m.GetValidationContextSdsSecretConfig() == nil {
    err := …{field: "ValidationContextSdsSecretConfig", reason: "value is required"}
```

**CONFIRMED LIVE** — the reference rejects a pure-inline CVC at config-validate time:

```
$ docker run --rm -v "$PWD:/probe:ro" envoyproxy/envoy:contrib-v1.37.2 \
    envoy --mode validate -c /probe/armA_pure_inline_cvc.yaml
Proto constraint validation failed (DownstreamTlsContextValidationError.CommonTlsContext: … caused by
CombinedCertificateValidationContextValidationError.ValidationContextSdsSecretConfig: value is required)
EXIT=1
```
Reproduced identically with `require_client_certificate: false`. **A CVC without an SDS half is unconfigurable in the reference.**

**⇒ The consequence is a NEW OBLIGATION, and it inverts the BRAINSTORM's question.** The BRAINSTORM asked *"accept as equivalent to a plain `validation_context`, or reject?"* — a question that presupposes the shape is legal. It is not.

**And envoy-go runs NO PGV validation anywhere.** Three independent confirmations, stated **repo-wide** rather than `internal/`-scoped: (i) `Validate()`/`ValidateAll()` call sites outside generated code, repo-wide non-test — **ZERO**; (ii) `protoc-gen-validate v1.2.1` is an **`// indirect`** module and **no `.go` file imports it**; (iii) no `interface{ Validate() error }` assertion exists outside vendored generated code. *(The draft cited this as `grep … internal/ cmd/` — a scope that **omits the top-level `validate/` package**, which is literally envoy-go's config-validation entry point. The conclusion held, but the same `internal/`-only blind spot is exactly what hid §3.7's severe defect. **State scopes repo-wide, or the grep that "proves" a negative is the grep that hides the counterexample.**)*

**⇒ envoy-go would happily accept both malformed shapes that the reference refuses — a silent cross-side divergence created by the row itself.**

**⇒ envoy-go MUST reject BOTH shapes explicitly** (§6, `E1`/`E2`) — mirroring the constraint the reference gets from PGV and envoy-go does not get at all. **The BRAINSTORM named NEITHER arm.** This is the `reference_strict_reject_sibling_typeurl_gap` lesson a second time in one row: an envelope-lift silently accepting what the reference refuses.

### 5.3 D-COMBVC-MERGERULES — only ONE of three rules is reachable, and NOT the one the proto names *(a NAMED COVERAGE BOUNDARY)*

envoy-go's honored `CertificateValidationContext` surface is **`trusted_ca` and nothing else** (§3.2 P1).

**The roster below is enumerated MECHANICALLY from the descriptor** — `(&tlsv3.CertificateValidationContext{}).ProtoReflect().Descriptor().Fields()` at v1.32.4, which reports **15 fields**, listed here in full. *(This SPEC's first draft carried **8** while claiming it was "re-enumerated in full from the generated type" — the exact accusation it levels at the BRAINSTORM one clause earlier, committed one clause later. The adversarial pass caught it. **Enumerate by reflection, never by reading.**)*

| f# | Field | Kind | envoy-go |
|---|---|---|---|
| 1 | `trusted_ca` | message | **HONORED — the only one** |
| 2 | `verify_certificate_hash` | repeated string | **rejected** |
| 3 | `verify_certificate_spki` | repeated string | **rejected** |
| 6 | `require_signed_certificate_timestamp` | message (`BoolValue`) | silently ignored (§3.3 row 8) |
| 7 | `crl` | message | silently ignored — a **SHARED** gap (§9) |
| 8 | `allow_expired_certificate` | bool | silently ignored |
| 9 | `match_subject_alt_names` (deprecated) | repeated message | **silently ignored** — not one of "the four" |
| 10 | `trust_chain_verification` | **enum** | silently ignored — **has `ACCEPT_UNTRUSTED`** ⚠️ |
| 11 | `watched_directory` | message | silently ignored (§3.4 — inert; the merge artifact) |
| 12 | `custom_validator_config` | message | **rejected** |
| 13 | `ca_certificate_provider_instance` | message | silently ignored |
| 14 | `only_verify_leaf_cert_crl` | bool | silently ignored |
| 15 | `match_typed_subject_alt_names` | repeated message | **rejected** |
| 16 | `max_verify_depth` | message (`UInt32Value`) | silently ignored |
| 17 | `system_root_certs` | message | silently ignored — empty msg ⇒ "use system roots" ⚠️ |

**The conclusion SURVIVES the correction, and the full enumeration is what proves it:** the repeated fields are exactly f2/f3/f9/f15 and the plain bools exactly f8/f14 — **all six already accounted for** — so **repeated-concatenate and bool-OR remain structurally unreachable**. The five fields the draft had missed (f10, f13, f16, f17, f11) are each message/enum-kind, so none reopens either rule.

**⚠️ But two of the missed fields are NOT inert as silent-ignores** — `trust_chain_verification: ACCEPT_UNTRUSTED` and `system_root_certs` are real trust-affecting surfaces on the **default half** that this row newly exposes to operators. They are **SHARED gaps** (the inline path ignores them identically), so §9 item 6's reasoning covers them — **but only now that they are named.** An unnamed silent-ignore is the failure this SPEC keeps re-learning.

**Repeated-concatenate and bool-OR are structurally UNREACHABLE** — envoy-go honors no repeated or bool field. The **one** reachable rule is the singular-`trusted_ca` case, and under Design A envoy-go **does not implement it via `MergeFrom` at all**; it achieves the identical observable **by construction** (§3.2's theorem).

**⇒ NAMED COVERAGE BOUNDARY, recorded in BEHAVIOR_CONTRACT.** The SPEC states plainly: **envoy-go does NOT implement `Message::MergeFrom()` semantics for CVC.** It implements a provably-equivalent substitution on the one-field surface it honors, with the empty-dynamic complement named as a departure (§3.9). *"We implement the documented merge semantics"* would be **precisely** the uncited-prose overclaim the BRAINSTORM's §11 warns about — and it would be **false**, not merely unsupported.

---

## 6. PARSE/BOOT-REJECT roster — all ADR-0080-distinct substrings

**Retained BYTE-IDENTICAL** (ADR-0080): `tls: upstream: combined_validation_context is not supported in phase 03` *(dead from every entry point — §3.7; retained deliberately, not because it fires)* · the same substring for `side == "downstream"` with a **nil provider**, which has **TWO live consumers — `NewQUICDownstreamConfig` AND `validate.Bootstrap`/`BootstrapFile`** (§3.7; the "QUIC only" claim is FALSE and was corrected after an adversarial pass). **The gate MUST keep the `provider != nil` conjunct or both leak in.**

**NEW — the two arms the BRAINSTORM never named (§5.2):**

| id | Shape | Required behavior | Why |
|---|---|---|---|
| **E1** | CVC with `default_validation_context == nil` | **REJECT**, distinct substring | PGV `value is required`; reference refuses at config-validate; envoy-go runs no PGV ⇒ would silently accept |
| **E2** | CVC with `validation_context_sds_secret_config == nil` (pure-inline) | **REJECT**, distinct substring | as above; also re-bases R7's "nil-check" into a reject |

Both MUST be rejected in **`commonTLSContextToConfig`** (so the shape dies at parse), and **E2's condition must also be honored by the `boot.go` pre-scan's third arm** so the two sites cannot disagree. Substrings must be ADR-0080-distinct from each other and from the existing roster; **the PLAN pins the exact strings and GREP-collision-checks them** (`reference_spec_drafted_identifier_collision_check`).

**RE-POINTED (§3.5)** — the four inline sub-field rejects must additionally cover `cvc.GetDefaultValidationContext()`: `custom_validator_config` · `match_typed_subject_alt_names` · `verify_certificate_hash` · `verify_certificate_spki`. All four are **BYPASSED today** ⇒ **each needs its OWN test**; none is currently proven.

**UNTOUCHED:** `parseValidationSecret`'s four served-half rejects; the `seen > 1` guard (§3.6).

**Recorded, NOT fixed (ambient, pre-existing):** the `validation_context_type` switch has **no `default:` arm**, so the two hidden+deprecated CVC-sibling arms (§5.1) pass unrejected. Pre-existing, not created here, and both arms are proto-hidden. **Out of scope**; noted so the PLAN neither "fixes" it nor claims the switch is exhaustive.

---

## 7. Stat surface — **+0** *(D-COMBVC-STATS — CONFIRMED)*

The phase-60.2 `sds.*` lifecycle counters are reused verbatim; the fetch path is unchanged. **Stat surface stays 1201.**

**Fuzz — +0** *(D-COMBVC-FUZZSEED — CONFIRMED)*: **SEEDS** to `FuzzTLSContextParse` (its `tls: ` prefix invariant already constrains the new branch — and now constrains E1/E2 too). **NO new fuzzer**; count stays **55**.

**D-COMBVC-FETCHTIMEOUT — CONFIRMED, not assumed.** `FetchInitialValidationContext` is reused verbatim under Design A, at the same bound, with no new knob ⇒ **the ADR-0280 boot-FAIL DEPARTURE extends UNCHANGED.** §3.9's departure is the *same* mechanism reached by a different input (a parse error rather than a timeout), which is why it lands inside ADR-0280's family rather than opening a new one.

---

## 8. Differential fixture taxonomy — **+1** *(D-COMBVC-FIXTURE / -OVERRIDE-OBSERVABLE / -STRUCTURAL)*

`test/fixtures/0109-xds-sds-combined-validation-context/` ⇒ fixtures **110 → 111**. Driver ~95% from `0108` (in-memory 5-artifact PKI, `mtlsEcho`, `structuralCheck`, `normalizeTLSErr`, per-side SDS servers, YAML templating). `sdsserver` **UNTOUCHED**. `BackendCount` ≥1 (`reference_differential_backendcount_min_one`).

**D-COMBVC-OVERRIDE-OBSERVABLE — the discriminating design, now CROSS-SIDE VALIDATED IN ADVANCE.** Serve **CA_X** over SDS; set `default_validation_context.trusted_ca` = **CA_Y** (a different, unserved CA). Then **client_X must be ACCEPTED and client_Y REJECTED** — proving the dynamic context replaced the specifier rather than unioning. `0108`'s two-arm `good=`/`bad=` verdict expresses this with **zero new observable machinery**.

**This SPEC has already OBSERVED the reference produce exactly this verdict** (§3.2 Design C refutation: `client_x` ACCEPTED, `client_y` rejected `UNKNOWN_CA`), so the fixture is known to be cross-side-satisfiable **before** it is written — and it is simultaneously the assertion that **refutes Design C**, making the fixture load-bearing for the design, not merely for the code.

**⚠️ Its D-COMBVC-EMPTY-DYNAMIC discrimination must be made DELIBERATE, not accidental** (the BRAINSTORM's own note). Under §3.9 the two sides **do not agree** on shape (a), so the fixture must **not** exercise an empty dynamic context on the cross-side path — that arm belongs in a **subject-side unit test** asserting the envoy-go boot-FAIL, with §3.9's departure recorded in BEHAVIOR_CONTRACT. **A cross-side fixture that wandered into shape (a) would fail legitimately** (`reference_differential_fixture_dispatch_constraint`: one fixture dir = ONE runner branch).

**D-COMBVC-STRUCTURAL — MANDATORY, and its necessity must be SHOWN, not asserted.** Phase 65 **DEMONSTRATED** that with `0108`'s `structuralCheck` disabled, a served-CA break ships **PASS**: both sides emit `good=REJECTED`/`bad=ACCEPTED` and `CompareBytes` compares EQUAL. **A pure-`CompareBytes` fixture ships green on a completely broken trust anchor.** Any CVC break changes BOTH sides identically ⇒ same trap (`reference_vacuous_break_receiver_normalizes`). **The IMPL must re-DEMONSTRATE this on `0109`** — disable the check, watch a broken-anchor break ship PASS, restore with `git restore` — and log it in PROGRESS.

**⚠️ A harness trap the probe hit, which the fixture inherits.** A **stale SDS server** left listening silently served the **previous arm's** config and nearly produced a **false "the reference honors it" divergence** — caught only because that arm's SDS log was empty (`bind: address already in use`). **Fresh-container-per-arm is NOT sufficient** (`reference_probe_fresh_container_per_arm` covers only the container). The driver's per-side SDS servers need the **same** discipline plus a **hard precondition assert that THIS arm's server actually served**. `0108` uses per-side servers already; the PLAN must confirm they cannot outlive an arm. **A memory update is owed** (§13).

---

## 9. Behavior-contract delta (ADR-0287 atomic landing at the IMPL)

The xDS/SDS section moves CVC **REJECTED → CONSUMED** (downstream + live provider + `require_client_certificate: true`), and records, each as a NAMED BOUNDARY:

1. The upstream + nil-provider reject (byte-identical substring; **TWO live consumers — QUIC and `validate`**, NOT one — §3.7).
2. **envoy-go does NOT implement `Message::MergeFrom()` for CVC** — a provably-equivalent substitution on the `trusted_ca`-only surface (§3.2/§5.3); only 1 of 3 documented rules is reachable, and repeated-concatenate + bool-OR are structurally unreachable.
3. **The empty-dynamic DEPARTURE (§3.9(a))** — the reference ACKs, falls back to the **default CA**, and **serves**; envoy-go **boot-FAILs** (ADR-0280 family). Phrased **per-shape**: (b)/(c) agree at the traffic level but diverge at the lifecycle level.
4. **`require_client_certificate: false` (§3.10)** — the reference honors the anchor (verify-if-presented, **PROBED**); envoy-go silently ignores it. Applies to **all three** paths (CVC, plain SDS-VC, fully-inline), not just CVC.
5. **E1/E2** — envoy-go rejects the two PGV-`required` violations explicitly, because envoy-go runs no PGV (§5.2).
6. **`crl` stays a documented SHARED gap** — the inline block does not check it either, so rejecting it on the CVC path would introduce a **NEW asymmetry**. CVC does not close it. **Recorded alongside it, now that §5.3's full enumeration names them: `trust_chain_verification` (incl. `ACCEPT_UNTRUSTED`), `system_root_certs`, `max_verify_depth`, `ca_certificate_provider_instance`, `match_subject_alt_names`** — all SHARED silent-ignores on the default half, all trust-affecting, none closed by this row.
7. **The `validate` path diverges (§3.7)** — the reference's `--mode validate` **ACCEPTS** a well-formed CVC config; envoy-go's `validate.BootstrapFile` **REJECTS** it post-lift with a now-misleading *"not supported in phase 03"*, because `validate.Bootstrap` passes a nil provider. Pre-existing for phase 65's SDS-VC arm; **named here because this row lifts CVC and would otherwise leave its own validate path silently contradicting the feature it just shipped.**

---

## 10. Test plan + per-task structure *(D-COMBVC-SPLIT — CONFIRMED: a SINGLE FLAT ROW)*

**~8–10 tasks across two production files** — within the BRAINSTORM's ~6–9 anticipation, nudged by E1/E2 (§6). **ADR-0045 escape-valve ARMABLE but UNCONSUMED:** there is no two-package surface that could strand a leg (`internal/xds` is untouched), and Design A **shrank** the row versus the BRAINSTORM's plan (the clone+merge and its bootstrap-mutation test are **gone** — R2). **NO split.** The PLAN decomposes; TDD per `superpowers:test-driven-development`.

Coverage the PLAN must produce (each an independent `Errorf` property — `reference_fatalf_makes_assertions_unreachable`):
- The CVC reject-flip (downstream + provider + require==true) — and the gate's **negative** arms: upstream, QUIC (**nil provider**), and provider==nil.
- **E1 + E2** — the two new rejects, each with a distinct ADR-0080 substring (§6).
- **The four re-pointed sub-field rejects on `default_validation_context` — four separate tests.** All are BYPASSED today ⇒ **a test that passes before the change proves nothing**; each must be shown RED first.
- The override observable at the unit level: served CA_X + default CA_Y ⇒ the pool contains **X and not Y**.
- **The equivalence theorem's boundary (§3.9)** — a served empty/degraded VC boot-FAILs on the envoy-go side, asserted **subject-side**, with the departure recorded (NOT a cross-side arm — §8).
- The boot pre-scan third arm + **`seen == 1`** (§3.6).
- Fuzz **seeds** to `FuzzTLSContextParse` (§7).
- Fixture `0109` (§8) + the **re-demonstrated** `structuralCheck` liveness break.

**Break-protocol discipline the PLAN must honor:** `-run 'TestDifferential/0109-xds-sds-combined-validation-context'` (`reference_differential_run_selector`); **`-count=1` on every deliberate break** (`reference_differential_break_protocol_count1`); a **non-compiling break proves NOTHING** — substitute a compiling equivalent, **REPORT the substitution**, record the TRUE result (`reference_plan_break_instructions_dont_compile`; phase 65 hit 2 non-compiling + 2 vacuous breaks); **a break that does NOT fire is a FINDING** — record it as an honest, UNCLAIMED coverage gap, do not route around it. **Confirm WHICH assertion fired** (`reference_deliberate_break_wrong_assertion`).

**`reference_sds_init_fetch_timeout_dial_budget_flake`** — if `TestProvider_FetchInitialCertificate_Timeout` fails under `-race`, it is **PRE-EXISTING on master** (observed once, 2026-07-16). Read the memory; do **NOT** reflex-classify it as a phase-66 regression. A SECOND occurrence justifies widening the budget.

---

## 11. Edit-site roster *(D-COMBVC-DOCSHAPE — RE-DERIVED against master `b9f5a1c8`, cited by SYMBOL)*

**Production — `internal/tls/config.go`:**
1. **`commonTLSContextToConfig`** — the `*tlsv3.CommonTlsContext_CombinedValidationContext` arm: reject → scoped no-op gated `side == "downstream" && provider != nil` (the landed phase-65 arm spells this as its De Morgan reject form; mirror it). **`[EDIT]`**
2. **`commonTLSContextToConfig`** — **E1 + E2**, the two new rejects (§6). **`[ADD]`**
3. **`commonTLSContextToConfig`** — the four inline sub-field rejects re-pointed to also cover `cvc.GetDefaultValidationContext()` (§3.5 — a correctness obligation; BYPASSED today). **`[EDIT]`**
4. **`NewDownstreamConfig`** — inside the `require_client_certificate` block, the if/else type-assert → a 3-way branch (SDS-VC / CVC / inline). **The CVC branch is `FetchInitialValidationContext` → `cfg.ClientCAs`. NO `proto.Clone`. NO `proto.Merge`. NO `loadTrustedCAPool` on the default half** (§3.2). **`[EDIT]`**

**Production — `internal/boot/boot.go`:**
5. **`NewSDSProvider`** — the pre-scan third arm for the CVC wrapper, honoring E2's condition (§3.6). **`[EDIT]`**

**Production — `internal/xds`:** **NONE. `[UNTOUCHED — the row's defining property, preserved by §3.2's theorem rather than by the BRAINSTORM's reasoning]`**

**Production — `validate/`** *(a TOP-LEVEL package, outside `internal/` — which every `internal/`-scoped grep in this lineage has missed; §3.7)*:
5b. **`validate.Bootstrap`** passes a **nil** `sdsProvider` to `boot.Construct`, making it the **second** live consumer of the nil-provider reject. **The PLAN must DECIDE and record: leave it rejecting (a named §9-item-7 boundary) or plumb a provider.** *Recommended:* **leave it**, record the boundary — plumbing SDS into a config-validator implies dialing a management server from `--mode validate`, which the reference does not do either. **`[DECIDE — no edit anticipated]`**

**⚠️ STALE/FALSE COMMENTS THE ROW MUST FIX (found this session; NOT in the BRAINSTORM's roster):**
6. **`NewDownstreamConfig`'s doc block** — claims *"the previously parse-rejected surfaces (SDS-bound secrets, custom_validator_config, match_typed_san, verify_certificate_hash/spki) **remain rejected** via the unchanged commonTLSContextToConfig pre-checks."* **SDS-bound secrets are NO LONGER rejected downstream** — untrue since phase 60.2 (certs) and phase 65 (validation contexts). **Comment rot that phase 66 makes worse if left.** **`[EDIT]`**
7. **`commonTLSContextToConfig`'s doc list** — lists *"validation_context_sds_secret_config set"* and *"combined_validation_context set"* flatly as forbidden. Half-false today; **fully false after this row**. **`[EDIT]`**
7b. **`commonTLSContextToConfig`'s SDS-arm comment — *"The guard below has exactly ONE live consumer: NewQUICDownstreamConfig"* — is FALSE** (§3.7): `validate.Bootstrap` is a second. **This comment took THIS SPEC in**, and a *third* comment in the same file (*"nil for upstream/**validate** callers"*) contradicts it. **The file disagrees with itself; the PLAN must fix the false one, not the correct one.** **`[EDIT]`**

*(These vindicate the BRAINSTORM's own "cite by SYMBOL" lesson from the other side: **prose rots exactly like line numbers do**, and a doc comment asserting a lifted reject is a `feedback_brief_citations_not_evidence` failure aimed at the next reader. Also confirmed stale, NOT fixed here — `parseValidationSecret`'s doc cites `config.go:234-245`/`:233-246` for the inline reject block; the block has moved. **Out of scope**; the PLAN may fold it in only if free.)*

**Test / harness:**
8. `internal/tls/config_test.go` — the reject-flip + the gate's negative arms + **E1/E2** + the **four** re-pointed sub-field rejects + the override observable. **`[EDIT/ADD]`**
9. `internal/boot/boot_test.go` — the pre-scan third arm + E2's boot-side condition + a `seen == 1` assertion. **`[EDIT/ADD]`**
10. Fuzz **seeds** to `FuzzTLSContextParse` (NO new fuzzer). **`[EDIT]`**
11. `test/helpers/sdsserver` — **`[UNTOUCHED]`**

**Fixture:**
12. `test/fixtures/0109-xds-sds-combined-validation-context/` — driver (~95% from `0108`), `envoy.yaml` + `envoy-go.yaml`, `expectations.yaml`, `README.md`. **`[ADD]`**

**Docs:**
13. `BEHAVIOR_CONTRACT.md` — the §9 delta (six items). **`[EDIT]`**
14. `DECISIONS.md` — ADR-0287 §Context **HERE** (ADR-0044); §Decision/§Consequences at the IMPL. **`[SPEC/IMPL]`**
15. `ROADMAP.md` — row 66 stays `in-progress`; **the deferred sentence is UNCHANGED** (§12). **`[IMPL]`**
16. `STATE.md` + `next-prompt.txt` — the stage roll (**TRACKED** — `reference_next_prompt_tracked_despite_gitignore`). **`[SPEC]`**

---

## 12. Sentinel maintenance — **NOTHING is owed**

**The live xDS deferred sentence (`ROADMAP.md:186`) is UNCHANGED by this row, at EVERY stage.** `combined_validation_context` was never in it — it is a §8-tier pickup from phase 65's deferred roster (the phase-64 precedent: that BRAINSTORM's commit left the Observability sentence byte-identical). **Do NOT fabricate a narrow.** Check (2) keeps printing three sentences; check (3) is unaffected (no new family). Row 66 keeps check (1) printing until its IMPL six-gate.

---

## 13. Deferred items *(carried; §8-tier)*

- **`xds-sds-upstream-server-cert`** — the **VALUE-level** constructibility cycle (`boot.NewSDSProvider` needs `grpcclient.New(cm)` needs the `*cluster.Manager` being built) + the listener-only pre-scan + the eager `upstreamCfg`/`extractH2Mode` coupling. The import graph is FINE, so `reference_xds_config_seam_transitive_cycle_guard` **passes while the row stays blocked**. Needs a boot-model reshape. **Mechanism recorded — do not re-derive.**
- **The empty-dynamic fallback (§3.9(a))** — **NEWLY EVIDENCED, not speculative.** Closing it requires **Design B** (§3.2): a message-returning `internal/xds` seam plus relocating `parseValidationSecret`'s rejects. Sized here; a real row.
- **`require_client_certificate: false` / verify-if-presented (§3.10)** — the reference behavior is now **PROBED and PINNED**. Must be lifted across **all three** paths (CVC, plain SDS-VC, fully-inline) in one row; needs `VerifyClientCertIfGiven` + a fetch-gate restructure.
- **SDS rotation** — and note §3.11's new evidence: the reference has an **implicit** watch on an SDS-delivered `trusted_ca{filename}` (reloads on atomic move, no `watched_directory` needed). Rotation is **not** only about honoring `watched_directory`. Scoping input recorded.
- **`watched_directory` sensitivity** — **UNESTABLISHED** (§3.4): no arm made it fire, so there is no positive control. A future rotation row must build one **first**.
- **The `validation_context_type` switch's missing `default:` arm** (§6) — ambient; both unhandled arms are proto-hidden + deprecated.
- **HTTP/3 `QuicProtocolOptions`** · **DataSource `environment_variable`** (blocked on the D-ENV-HARNESS seam SPEC-63 declined) · **tracing `custom_tags` `metadata`** (~19 call sites) · **the compose-two edge** · **SDS `initial_fetch_timeout`/backoff edges** (the reference side is a **proto reading, not a probe**) · **`crl`** (never sized standalone) · **the repeated-concatenate + bool-OR rules** (structurally unreachable, §5.3) · **the `ssl` stat family** (framework surgery, ADR-0286 C3) · **CDS/EDS · LDS/RDS · ADS · Delta xDS · RTDS · `google_grpc`** · **gRPC / Runtime / WASM** (family openers). All carry forward.

**Memory updates owed at the IMPL:**
- **(i) NEW** — *a driver-owned probe/SDS server needs fresh-per-arm discipline **plus a served-this-arm precondition assert**; `reference_probe_fresh_container_per_arm` covers only the CONTAINER.* A stale server silently served the previous arm's config and **nearly produced a false divergence report** (§8).
- **(ii) EXTEND `reference_quoting_is_not_executing`** — *executing the semantics is necessary but **not sufficient: re-derive the SEAM's signature too**.* Phase 66's BRAINSTORM ran `proto.Merge` exactly as instructed and **still planned unbuildable code**, because it CITED `FetchInitialValidationContext`'s return type (§3.1). **And a probe must DISCRIMINATE** — R11's one-input `BoolValue` probe *ran* and still shipped a false negative (§3.3).
- **(iii) NEW / EXTEND `feedback_brief_citations_not_evidence`** — ***a landed CODE COMMENT is not evidence.*** It is uncited prose wearing the authority of proximity. §3.7's severe defect came from one, in a file that **contradicts itself** (two comments disagree; the SPEC trusted the false one). **RE-DERIVE control-flow claims from the call graph, never from the comment next to them.**
- **(iv) NEW** — *an `internal/`-scoped grep MISSES the top-level `validate/` package.* It hid both §3.7's severe defect and §5.2's scope understatement. **State negative-proving greps repo-wide.**
- **(v) NEW** — *the reference's PGV layer can foreclose a hazard that Go-side `proto.Merge` reasoning predicts* (§3.9(c)). **Probe the reference before recording a Go-derived divergence** — reasoning from `proto.Merge` alone would have invented one here.

---

## 14. ADR continuity — the ADR-0287 §Context DRAFT (anchored here; full entry at the phase-66 IMPL)

> **ADR-0287 — xDS SDS `combined_validation_context` (the FOURTH xDS-family row; the THIRD SDS applier row and the FIRST adding ZERO new symbols to `internal/xds`; a SINGLE FLAT ROW, the SOLE leg — FLIPS ROW 66 `done`; the xDS family STAYS OPEN).**
>
> **§Context (drafted at the phase-66 SPEC per ADR-0044).** Phase 60.1 built the SDS discovery substrate (ADR-0278); phase 60.2 wired the FIRST applier (downstream server cert, ADR-0280); phase 65 proved the substrate carries a SECOND resource type (`validation_context` → `*x509.CertPool` → `ClientCAs`, ADR-0286). `combined_validation_context` is the sibling oneof arm composing that SDS-delivered context with an **inline** `default_validation_context`; its reject is the second arm of `commonTLSContextToConfig`'s `validation_context_type` switch. The row lifts that reject for **downstream + live provider + `require_client_certificate: true`** only. The upstream arm is **structurally dead** behind `NewUpstreamConfig`'s earlier `trusted_ca is required` refusal (the oneof makes `GetValidationContext()` nil), and the retained BYTE-IDENTICAL substring (ADR-0080) keeps firing for `side == "downstream"` with a nil provider — which has **TWO live consumers, `NewQUICDownstreamConfig` and `validate.Bootstrap`**, so the gate must retain its `provider != nil` conjunct. *(A landed code comment asserts "exactly ONE live consumer: NewQUICDownstreamConfig"; it is **false**, a second comment in the same file contradicts it, and the SPEC inherited the false one until an adversarial pass executed `validate.BootstrapFile` on a plain TCP listener and refuted it. The comment is corrected by this row.)* A consequence is named as a boundary: the reference's `--mode validate` **accepts** a well-formed CVC config while envoy-go's `validate` path **rejects** it, having no provider to fetch with.
>
> The proto specifies CVC's composition as `Message::MergeFrom()`. **envoy-go does NOT implement those semantics, and the SPEC records why rather than implying fidelity.** `internal/xds`'s `SecretProvider.FetchInitialValidationContext` returns an `*x509.CertPool`: `parseValidationSecret` consumes the `CertificateValidationContext` message and discards it, so **no message crosses the seam and `proto.Merge` has no operand** — the merge the BRAINSTORM planned is unbuildable without a new `internal/xds` symbol. Three designs were weighed. A **CA-pool union** is REFUTED by live probe (the reference accepts the SDS-served CA and rejects the default's with `UNKNOWN_CA`; a union would accept both) and by the proto's own singular *"**The resulting** `CertificateValidationContext`"*. A **message-returning seam** buys exact fidelity but breaks the row's zero-new-symbol property and forces `parseValidationSecret`'s rejects to relocate. The row therefore adopts **pool substitution**: the SDS-delivered pool wins and `default_validation_context.trusted_ca` is not read — justified by an **equivalence theorem** proven from four premises, each executed or re-derived rather than cited: envoy-go honors only `trusted_ca`; `proto.Merge` recurses into it as a message while its `specifier` **oneof** replaces (OBSERVED); a successful fetch **guarantees** that specifier is set (`dataSourceBytes` errors otherwise); therefore the merged pool ≡ the SDS pool exactly, and the merge's only surviving artifact — a hybrid `DataSource` carrying the default's `watched_directory` — is inert on envoy-go's surface. **The theorem's complement is the row's ONE named departure:** when the served context carries no usable `trusted_ca`, the reference ACKs, falls back to the **default CA**, and **serves traffic** (PROBED), while envoy-go boot-FAILs — the same ADR-0280 boot-FAIL family reached by a parse error rather than a timeout.
>
> Two further obligations the envelope-lift creates, **neither anticipated by the BRAINSTORM**: the four inline sub-field rejects are **BYPASSED** under CVC (the oneof makes `GetValidationContext()` nil), so they must be re-pointed at `default_validation_context` or the row silently accepts what envoy-go cannot honor; and **both** CVC halves are PGV-`required` — the reference refuses a pure-inline or default-less CVC at config-validate, while **envoy-go runs no PGV anywhere** (repo-wide: zero `Validate()`/`ValidateAll()` call sites outside generated code; `protoc-gen-validate` is an `// indirect` module no `.go` file imports), so both shapes need explicit rejects. `internal/boot`'s `NewSDSProvider` pre-scan — which today asserts one oneof type and returns a **silent `(nil, nil)`** — grows a third arm, without which a CVC listener **boot-FAILs with the retained reject**, a misleading message rather than a silent trust-anchor bypass. `internal/xds`, `parseValidationSecret`, `FetchInitialValidationContext`, and `test/helpers/sdsserver` are **BYTE-UNTOUCHED**.
>
> *(§Decision + §Consequences land at the phase-66 IMPL.)*

---

## 15. Exit — counts + ROADMAP/STATE at SPEC-DONE

**Counts UNCHANGED at this SPEC (docs-only; re-verified against master `b9f5a1c8` this session, `git fetch` first):** fixtures **110** · fuzzers **55** · stat surface **1201** · BackendKind **38** · DECISIONS tail **ADR-0286** (next-free **ADR-0287**) · go.mod modules **2** (repo total 67).

**Anticipated at the phase-66 IMPL:** fixtures **110 → 111** (`0109-xds-sds-combined-validation-context`) · stat surface **1201 (+0)** · fuzzers **55 (+0)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0286 → ADR-0287** · go.mod **2 (+0)** · ZERO new packages · **ZERO new symbols in `internal/xds`**.

**ROADMAP row 66 STAYS `in-progress`** (ADR-0106 — the IMPL six-gate flips it). **The deferred sentence is UNCHANGED** (§12). **Sentinel re-run mechanically this session — does NOT fire; `stop` NOT created.**

**Next → the phase-66 PLAN.** Its inheritance, stated plainly:

- **Design A is ADOPTED and its theorem is proven and adversarially attacked (§3.2).** The PLAN must **NOT** re-open the seam, and must **NOT** write `proto.Merge`/`proto.Clone` — **which the BRAINSTORM's §6 still instructs, and which cannot compile** (§3.1). **The BRAINSTORM is superseded on the implementation; read it for the subject and the envelope only.**
- **The PLAN's own obligations:** the **two rejects the BRAINSTORM never named** (E1/E2, §6) · the **four BYPASSED sub-field rejects**, each shown **RED first** — they pass today for the wrong reason, so a green test proves nothing (§3.5) · the **`provider != nil` conjunct**, now known to gate **two** consumers (§3.7) · the **per-shape** departure phrasing (§3.9 — *never* "envoy-go rejects where the reference rejects") · the `structuralCheck` **re-DEMONSTRATION** (§8) · **P5's comment** at the apply-point (§3.2).
- **A SPEC is not evidence either — and this one proves it about itself.** Phase 65's IMPL found eleven defects in a PLAN that had itself corrected five SPEC defects. **This SPEC's own first draft carried FOUR defects, one SEVERE, every one of them in a section where it was most confident it had already caught the error** (§1.2). Its corrections are recorded in place, with their mechanisms, precisely so the PLAN can attack them rather than inherit them. **RE-DERIVE this document; do not execute it.** Where it cites, go look. Where it claims a control-flow fact, walk the call graph. **Default to REFUTED.**
