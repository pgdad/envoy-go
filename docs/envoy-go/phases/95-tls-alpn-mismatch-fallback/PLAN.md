# Phase 95 — `tls-alpn-mismatch-fallback` — PLAN

Lifecycle state **2 -> 3**. Predecessors: `BRAINSTORM.md` (314 lines) at `7f568db2`, the router correction at `abc309d7`, `SPEC.md` (516 lines) at `9ed6a620`. This stage discharges the nine owed items of `SPEC.md` §15 and refutes its predecessors **seventeen** times by execution.

**Charter, unchanged from `ROADMAP.md:157`:** on the TCP downstream path, a client that offers a NON-EMPTY ALPN list overlapping none of the chain's `alpn_protocols` must COMPLETE the handshake with no protocol selected — matching the pinned reference — instead of aborting with `no_application_protocol`.

## Global Constraints

- ⚠️ **This is a PLAN. It lands NO code.** Every prototype, patch and negative control described below was applied in a throwaway worktree, measured, and reverted under a `sha256sum -c` guard. Four measurement worktrees finished with `git status --porcelain --untracked-files=all` printing NOTHING.
- ⚠️ **`-count=1` is not optional** anywhere. The differential subject is a SUBPROCESS and the test cache serves a stale PASS.
- ⚠️ **`go test` without `-v` prints zero `=== RUN`.** `RUN=0` beside `RC=0` is a VACUOUS GREEN. A `-run` selector matching nothing prints `[no tests to run]` and **EXITS 0** — assert the selector matched.
- ⚠️ On `-v` output use `grep -cE '^(FAIL|--- FAIL)|^ *--- FAIL'`; unanchored `grep -c FAIL` reads nonzero on a fully green tree. `grep -c` on zero matches prints `0` **and exits 1** — capture with `v=$(cmd || true)`, never `$(cmd || echo 0)`.
- ⚠️ `rc=$?` after a pipe returns the LAST command's status — use `out=$(…); rc=$?` or `PIPESTATUS[0]`. `INNER_EXIT` does not exist in this repo.
- ⚠️ **`gofmt -l` never exits non-zero — gate on OUTPUT.** `golangci-lint`'s misspell runs in **locale US**: sweep British spellings in `.go` comments before the gate. Markdown prose may use them freely.
- ⚠️ **A build is not evidence the edit landed.** At the phase-95 BRAINSTORM a prototype built, booted and SERVED with its callback never installed. **Assert the symbol, then DRIVE the arm.**
- ⚠️ **A green test run is not evidence a site is exercised** — prove it with a `panic()` reachability control.
- ⚠️ **PATHSPEC-SCOPE and RANGE-SCOPE every symbol assertion and every `sed`.** §0.6 and §0.10 below are both instances of an anchor that is not unique.

---

## 0. What this PLAN refuted by execution — SEVENTEEN claims

Escalation across this phase: BRAINSTORM **nine** · SPEC **eleven** · **PLAN SEVENTEEN**. Four are load-bearing: §0.1 confirms a security claim the SPEC only reasoned about, and §0.2, §0.3 and §0.4 each identify a pin that, written as specified, **fails against a CORRECT implementation or cannot fail at all**.

### ⚠️ 0.1 — `SPEC.md` §0.1's AUTHENTICATION BYPASS IS **CONFIRMED BY EXECUTION**, AND IT IS WORSE THAN THE SPEC STATES: THE BYPASSED CLIENT IS NOT MERELY ADMITTED, IT IS **SERVED**

The SPEC asserted the bypass from mutation order and never ran it. NC 7 was written deliberately (§0.1's pre-built snapshot at the `:57` anchor) and driven against a live mTLS listener.

**The mechanism, by direct instrumentation** — `require_client_certificate: true` + inline `trusted_ca` + `alpn_protocols: ["h2","http/1.1"]`:

```
INSTALL-TIME (config.go:57 anchor):  cfg.ClientAuth=NoClientCert                 cfg.ClientCAs==nil? true
HANDSHAKE-TIME (inside callback):    cfg.ClientAuth=RequireAndVerifyClientCert   cfg.ClientCAs==nil? false
  PREBUILT alt.ClientAuth=NoClientCert               alt.ClientCAs==nil? true
  PER-HANDSHAKE clone.ClientAuth=RequireAndVerifyClientCert  clone.ClientCAs==nil? false
```

§0.1's prediction is exactly right, and the two configs are shown side by side at the same instant.

**The consequence, measured** — a client presenting **NO client certificate**, at BOTH TLS versions:

```
arm=b_withheldcert_mismatch/TLS1.3 hsCompleted=true served=true negotiated="" certSent=false hsErr=<nil> ioErr=<nil>
  ssl leaves: handshake=1 fail_verify_error=0 fail_verify_no_cert=0 no_certificate=1 connection_error=0
arm=b_withheldcert_mismatch/TLS1.2 hsCompleted=true served=true negotiated="" certSent=false hsErr=<nil> ioErr=<nil>
```

⚠️ **`served=true` means a full application round trip through the proxy**, not a completed handshake — the distinction `reference_openssl_sclient_answers_handshake_not_served` exists to force. **Mandatory mTLS silently became anonymous access.** Under the bug arm (a) reddens for a SECOND reason: `certSent=false` — the server never even REQUESTS a certificate, so a client holding a valid one silently sends nothing.

Reachability proven with a `panic()`, not a green run (SPEC §15 item 6): the callback's mismatch branch panicked on demand — `panic: P95-M4 REACHABILITY CONTROL: the callback's MISMATCH branch was REACHED`, RC=1.

**Three tree states, four arms** (state 2 = the correct §4 code, applied byte-for-byte from the SPEC):

| arm | (1) TIP | (2) correct §4 | (3) §0.1 bug |
|---|---|---|---|
| control — valid cert, `["http/1.1"]` | PASS · `negotiated="http/1.1"` | PASS | PASS |
| a — valid cert, `["bogus/9"]` | **FAIL** · `no application protocol`, `connection_error=1` | **PASS** · served, `negotiated=""`, `certSent=true` | **FAIL** · served but `certSent=false` |
| b — no cert, `["bogus/9"]`, TLS1.3 | **FAIL** (ALPN error fired, not cert) | **PASS** · not served, `fail_verify_no_cert=1` | **FAIL — BYPASS** |
| b — no cert, `["bogus/9"]`, TLS1.2 | **FAIL** (same) | **PASS** · not served, `fail_verify_no_cert=1` | **FAIL — BYPASS** |

State 2 whole test: `RC=0`, `RUN=5`, all arms PASS. **§4's code compiles and passes VERBATIM and needs no new import.**

### ⚠️ 0.2 — `SPEC.md` §12 CELL 13 IS A **VACUOUS GUARD**: REMOVING `alpn_protocols` FROM THE SUBJECT SIDE LEAVES THE FIXTURE **GREEN**

§12 cell 13 specifies: *"remove `alpn_protocols` from ONE side only ⇒ RED cross-side — proves both YAMLs were edited."* **Executed, both directions, on a booted pair. It does not work.**

| direction | result |
|---|---|
| removed from **`envoy-go.yaml`** (subject) only, arm (vi) present, pins `3`/`2` | `RC=0`, `RUN=2`, `FAILCOUNT=0` — **GREEN.** Both sides read `conn_err=3, handshake=2`; the subject logged `handshake OK negotiated=""` and completed the echo. **The NC DOES NOT FIRE.** |
| removed from **`envoy.yaml`** (reference) only | `RC=1`, `FAILCOUNT=5` — RED, but **byte-identical to the un-neutralised run**. Red for the tip bug (cell 11), not for the missing YAML edit. The reference read `ce=3, hs=2` **with and without** `alpn_protocols`. **CONFOUNDED.** |

**Why it is structurally vacuous, not merely mis-specified.** None of the six arms can observe `alpn_protocols` through `ssl.connection_error` / `ssl.handshake`. Arms (i)-(v) send no ALPN extension at all; arm (vi) sends a non-overlapping one. **A server carrying `alpn_protocols` plus the §4 fallback and a server carrying no `alpn_protocols` at all produce the SAME outcome on every one of those six arms.** The measured proof of the equivalence is decisive: the subject WITHOUT `alpn_protocols` reads **`3`/`2`** — which is exactly the post-fix target — so the "neutralised" configuration is **indistinguishable from the fixed one**. `topic_gate_hygiene`'s vacuous-guard pattern, and it would have shipped as a roster cell that can never fail.

**The repair, MEASURED not proposed.** An arm offering an **OVERLAPPING** list and asserting the **client-visible negotiated protocol** does discriminate — probe arm (vii), offer `["h2","http/1.1"]`:

| configuration | reference | subject |
|---|---|---|
| both YAMLs carry `alpn_protocols` | `negotiated="h2"` | `negotiated="h2"` |
| removed from the **subject** YAML only | `negotiated="h2"` | **`negotiated=""`** — while stats stayed `3`/`2` on both sides and `FAILCOUNT=0` |

⇒ **the stat counters cannot see the YAML edit; the negotiated protocol can.** The PLAN either lands an overlap arm pinning `ConnectionState().NegotiatedProtocol` per side, or records cell 13 as unfireable. It does NOT ship it as written.

### ⚠️ 0.3 — SPEC §5.3's "ASSERT THE CLIENT-CERTIFICATE SIGNATURE" INSTRUCTION PRODUCES A **FALSE RED AGAINST THE CORRECT IMPLEMENTATION**

§5.3 mandates: *"arm (b)'s failure must carry the client-certificate signature, and must NOT be `no_application_protocol`."* Taken literally on the client-visible error text, **that assertion fails against the CORRECT code** — measured, not reasoned:

| TLS version | client-visible error under the CORRECT §4 implementation |
|---|---|
| **1.3** | `remote error: tls: certificate required` |
| **1.2** | `remote error: tls: handshake failure` — **no `certificate` substring at all** |

Cause, read not assumed: `crypto/tls/handshake_server.go:966-971` sends `alertCertificateRequired` only at TLS1.3 and `alertHandshakeFailure` otherwise. A `strings.Contains(err, "certificate")` assertion on arm (b) is therefore **version-dependent**, and the false red was reproduced.

⚠️ **The robust, version-invariant discriminator is SERVER-SIDE, and it is a TRIPLE:** `ssl.fail_verify_no_cert == 1` **and** `ssl.connection_error == 0` **and** `ssl.handshake == 0`. That says which error fired AND which did not (`next-prompt.txt` §7g) without depending on an alert code the negotiated version selects. **The PLAN specifies the counter triple; a client-string assertion, if kept at all, is scoped to TLS1.3 explicitly.**

**A FIFTH signature of the bypass, worth pinning:** under the bug arm (b) reads **`no_certificate=1` alongside `handshake=1`** — a "handshake succeeded with no client certificate" pair that **cannot occur** on a `require_client_certificate: true` chain. It belongs in arm (b)'s `wantLeaves` map.

### ⚠️ 0.4 — SPEC §5.2's "`handshake == 1`, REST 0" IS REFUTED: EVERY COMPLETING HANDSHAKE ON THAT HELPER ALSO BOOKS `ssl.no_certificate`

§5.2's table gives rows 7 and 8 `connection_error 0` and *"`handshake == 1`, rest 0"*, and §5.2 further specifies a `wantLeaves map[string]int64` **merged over a zero default**. Measured at the tip:

| arm | listener ALPN | offer | handshake | negotiated | `connection_error` | `handshake` | **`no_certificate`** |
|---|---|---|---|---|---|---|---|
| **row 7** | `["h2","http/1.1"]` | `["bogus/9"]` | **FAILED** — `no application protocol` | — | **1** | **0** | 0 |
| **row 8** | `["h2"]` | `["http/1.1"]` | OK | `""` | 0 | **1** | **1** |
| overlap | `["h2","http/1.1"]` | `["h2"]` | OK | `"h2"` | 0 | 1 | **1** |
| absent | `["h2","http/1.1"]` | *(none)* | OK | `""` | 0 | 1 | **1** |

**Row 7 IS red at the tip and row 8 IS green at the tip — SPEC §15 item 4's prediction HOLDS on both.** But *"rest 0"* does not: `ssl.no_certificate` reads **1** on every arm whose handshake COMPLETES. It is structural, not a flake — `startOneWayTLSListener`'s own doc comment records that the listener sends no `CertificateRequest`, so every completed handshake books it. The six existing table rows are unaffected because **none of them completes a handshake**; rows 7 and 8 are the first that do. After the fix row 7 completes too and also reads `no_certificate == 1` (measured).

⇒ ⚠️ **`wantLeaves` for rows 7 and 8 must be `{handshake: 1, no_certificate: 1}`. A map written from the SPEC's stated figure goes RED against a CORRECT implementation** — the same false-red shape as §0.R's TLS1.2 error string, found independently in a different layer.

### ⚠️ 0.5 — `SPEC.md` §7's COMMENT ROSTER NAMES ONE MEMBER OF A NINE-MEMBER CLASS, AND TWO OF THE MISSED SITES ARE NOT HISTORY AT ALL

`reference_measured_prototype_is_a_lower_bound` fires for a **THIRTEENTH** consecutive row. §0.6 caught the SPEC's predecessor under-enumerating by ONE comment site; the SPEC then under-enumerated the same way, one class wider.

`§7` names `internal/listener/manager.go:138` and rules it *"factually correct as history and must NOT be reverted"*. Enumerated by grepping the literal symbol outward, case-insensitively, over `internal/*` and `test/*`, **that class has NINE members and §7 names one:**

| site | text class | after the row |
|---|---|---|
| `internal/listener/manager.go:138` | history — **named by §7** | append, do not revert |
| `internal/bootstrap/bootstrap.go:84` | history (Task 10 removed the implicit SNI extraction) | **MISSED** — guarded non-site |
| `internal/listener/manager_test.go:1023` | history (`selectByServerNameFromMgr` is the replacement) | **MISSED** — guarded non-site |
| `internal/listener/manager_test.go:1030` | history (*"that callback no longer exists"*) | **MISSED** — guarded non-site, **and it is the site that PROVES the two below are false** |
| `internal/listener/manager_test.go:1276` | history (*"the pre-refactor … callback emitted natively"*) | **MISSED** — guarded non-site |
| `internal/listener/manager_test.go:1732` | history (*"The pre-Task-10 dispatch path used a hard-wired … callback"*) | **MISSED** — guarded non-site |
| `test/fixtures/0002-tls-tcp/driver/driver.go:196` | history (*"Task 10 deleted the legacy … shortcut"*) | **MISSED** — guarded non-site |
| `test/fixtures/0002-tls-tcp/driver/driver.go:248` | history | **MISSED** — guarded non-site |
| `test/fixtures/0002-tls-tcp/envoy-go.yaml:14` | history | **MISSED** — guarded non-site |

⚠️ **AND TWO SITES ARE NOT HISTORY — THEY ARE BARE PRESENT-TENSE CLAIMS THAT ARE ALREADY FALSE AT THE TIP, AND THIS ROW MAKES THEM WORSE BY RESURRECTING THE SYMBOL THEY NAME:**

| site | verbatim | status |
|---|---|---|
| `internal/listener/tls_handshake_negative_test.go:27` | *"envoy-go reaches the same outcome via SelectChain returning an error from the `GetConfigForClient` callback, which the Go TLS server turns into a handshake abort."* | **ALREADY FALSE** — `manager.go:138` and `manager_test.go:1030` both record that this callback was DELETED at phase 07.2 Task 10 and that chain selection now happens BEFORE the handshake. ⚠️ **It is in the ROSTER'S OWN FILE**, 58 lines above the `:85-87` block §0.6 caught and 86 above the `:113-120` block §7 names — the SPEC enumerated two comment blocks in this file and walked past a third. |
| `internal/listener/manager_test.go:1086` | *"verifies that two TLS chains with distinct `server_names` route correctly via `GetConfigForClient`."* | **ALREADY FALSE**, same reason, and refuted BY ITS OWN FILE 56 lines earlier at `:1030`. |

**Why this is load-bearing rather than tidy.** Before this row the tree contains NO `GetConfigForClient` callback at all, so a stale mention is merely dead history. **This row installs the first real one since phase 03** — and it does ALPN fallback, never SNI dispatch, and never returns an error. After the row, `tls_handshake_negative_test.go:27` and `manager_test.go:1086` read as descriptions of the callback the row just added, and both descriptions are wrong in the most confusing possible way: one says it returns an error to abort a handshake (§2.3: **the phase-95 callback has no error path at all**), the other says it routes on SNI. **Both must be rewritten in the same commit as the code, and the nine history sites must be explicitly guarded so a sweep does not "correct" them.**

### ⚠️ 0.6 — AN UNSCOPED GREP FOR THE RETURN STATEMENT READS **SEVEN**, AND THE SEVENTH IS THE QUIC RETURN THE ROW MUST NEVER TOUCH

`SPEC.md` §3.1 justifies the entry install with: *"`awk` over `42-259` finds exactly six `return &DownstreamConfig{...}` statements … and every one is byte-identical: `return &DownstreamConfig{TLSConfig: cfg}, nil`."* Both halves need qualifying, and the second is the dangerous one.

**Measured at this tip, three matchers over `internal/tls/config.go`, three different figures:**

| matcher | reads |
|---|---|
| exact 3-tab literal `^\t\t\treturn &DownstreamConfig{TLSConfig: cfg}, nil$` | **2** |
| anchored, no trailing comment, any indent | **5** |
| prefix form, allows a trailing comment, any indent | **7** |

- **"Byte-identical" is false as written.** `:220` and `:248` carry the trailing comment `// false/absent + no anchor -> NoClientCert`. The *statement* is identical at all six; the *line* is identical at four. An IMPL that verifies "six returns" with a literal-line grep reads **2** or **5**, never 6, and may conclude the anchor analysis is wrong.
- ⚠️ **The unscoped form reads SEVEN, and the seventh is `:290` — the `NewQUICDownstreamConfig` return.** `NewDownstreamConfig` spans from `:42`; `NewQUICDownstreamConfig` begins at `:268`. §3.1's figure of six is correct ONLY because it was RANGE-scoped to `42-259`, which the SPEC states but does not flag as load-bearing. **It is load-bearing:** §2.2 proves that emptying `NextProtos` on the QUIC path would make `negotiateALPN`'s RFC 9001 §8.1 guard VACUOUS and silently disable a mandated rejection. An IMPL that greps unscoped, finds seven, and treats them as one family installs the fallback on the one site the whole design forbids.

⚠️ **`next-prompt.txt` §3 says PATHSPEC-SCOPE every symbol assertion. This is the same hazard one level finer: RANGE-scope it too, and NAME THE MATCHER.** The PLAN's stable-anchor table therefore anchors on `func NewDownstreamConfig(` / `func NewQUICDownstreamConfig(` boundaries, never on the return text.

**What DOES hold, re-verified independently:** `cfg` is **never rebound** anywhere in `42-262` — an `awk` for `cfg` on the left of `=` or `:=` in that range prints NOTHING. Only its fields are mutated (`:94`, `:95`). So the single entry mutation at `:57` provably reaches all six exits, which is the property the placement actually rests on.

### 0.7 — §6.2's ARM ARITHMETIC IS NO LONGER A PREDICTION: SIX ARMS, TWO SIDES, MEASURED

`driver.go:73` warns its own pins are arm arithmetic a sixth arm invalidates, and SPEC §6.2 labels every figure in its bullet a PREDICTION. Booted both sides against the pinned reference (`envoyproxy/envoy@sha256:7edd5b0fd763…`, verified against `ENVOY_TARGET.md:3-4`), scraping each side's own `/stats/prometheus` after every `record()`:

| # | arm | REF Δce | REF Δhs | SUBJ Δce | SUBJ Δhs |
|---|---|---|---|---|---|
| (v) | valid + client cert | +0 | +1 | +0 | +1 |
| (i) | bad TLS version (max TLS1.1) | +1 | +0 | +1 | +0 |
| (ii) | plaintext HTTP | +1 | +0 | +1 | +0 |
| (iii) | garbage bytes | +1 | +0 | +1 | +0 |
| (iv) | clean FIN, 0 bytes | +0 | +0 | +0 | +0 |
| **(vi)** | **valid cert + ALPN `["bogus/9"]`** | **+0** | **+1** | **+1** | **+0** |
| | **TOTAL** | **3** | **2** | **4** | **1** |

`ssl.fail_verify_error` and `ssl.fail_verify_no_cert` stayed **0** on both sides through every arm of every run.

- **SPEC §6.2's prediction is CONFIRMED:** `wantConnectionError` **STAYS 3**, `wantHandshake` **1 -> 2**.
- **SPEC §12 cell 11 is CONFIRMED:** the subject at the tip contributes `+1`/`+0` against a `+0`/`+1` pin and fails with exactly two errors — `subject: ssl.connection_error = 4, want 3` and `subject: ssl.handshake = 1, want 2` (`runner_test.go:1354`).
- ⚠️ **§2.1 row 3 is now confirmed END-TO-END, not just at the handshake.** The reference arm (vi) logged `handshake OK negotiated=""` **and completed the echo round-trip** — the connection was SERVED, not merely handshaken. (`reference_openssl_sclient_answers_handshake_not_served`: this is the distinction that turned "no alert" into "200 OK" at the BRAINSTORM.)
- **Adding `alpn_protocols` moves arms (i)-(v) by ZERO on BOTH sides** — the no-`alpn` and with-`alpn` five-arm runs produced **byte-identical** delta sequences per side. `driver.go:78-83`'s existing table is unchanged and correct. §6.2's "expected to, but expected is not a measurement" is now a measurement.
- **§6.3's over-firing control CONFIRMED:** arm (vi) driven twice gives the reference `+0/+1` then `+0/+1` ⇒ **`handshake +2`, `connection_error +0`**, green under pins `3`/`3`. The subject over-fires symmetrically the wrong way (`+1/+0` twice ⇒ `5`/`1`). **If the control lands as a seventh permanent arm the pins become `3`/`3`, not `3`/`2`.**

### ⚠️ 0.8 — THE `panic()` REACHABILITY CONTROL FIRES ON **TWO** OF FOUR DRIVES, AND THAT RETIRES ROW 8's POST-FIX VALUE

SPEC §15 item 6 demands reachability be proven by `panic()`, not by a green run. Done — and there is no `recover()` anywhere in non-test `internal/listener` or `internal/tls`, so a panic aborts the binary decisively.

| drive | anchored `^panic:` | mismatch branch entered? |
|---|---|---|
| row 7 — `["bogus/9"]` vs `["h2","http/1.1"]` | **1** | **YES** |
| row 8 — `["http/1.1"]` vs `["h2"]` | **1** | **YES** |
| overlapping — `["h2"]` | 0 | no |
| absent offer | 0 | no |

The stack confirms the §2.3 dispatch anchor is real at go1.26.7: `installALPNMismatchFallback.func3` ← `crypto/tls.(*Conn).readClientHello` ← `handshake_server.go:169`.

⚠️ **Row 8 panicking is PREDICTED by §3.3 (the predicate deliberately subsumes `http11fallback`) and is now MEASURED — but it has a consequence the SPEC does not draw.** After the fix our callback intercepts row 8 BEFORE the stdlib's `http11fallback` ever runs. So **post-fix, row 8 is green under both a correct fix and a stdlib-only "fix"; only row 7 discriminates.** Row 8's entire value as a control is **PRE-FIX** (it is what makes §12 cell 5 meaningful), and the post-fix guard against over-firing is the §5.1 control arm, not row 8. **The PLAN states this rather than leaving a reader to infer row 8 guards something it cannot.**

### ⚠️ 0.9 — §12 CELL 10 IS INVISIBLE UNLESS §5.4 RE-READS THE BASE CONFIG **AFTER** DRIVING THE CALLBACK

Cells 2, 9 and 10 were each applied against the patch and each went RED as designed. But cell 10 (empty `cfg.NextProtos` in place instead of on a clone) fired **only** the base-config assertion:

```
zzm2guard_test.go:119: base NextProtos AFTER callback = [], want [h2 http/1.1] — the base config was mutated
```

**`alt.NextProtos == nil`, `alt.GetConfigForClient == nil`, `alt.ClientAuth` and `alt.ClientCAs` ALL STAYED GREEN under cell 10**, because `Clone()` faithfully copies the already-emptied slice. ⇒ **If §5.4's third bullet is written without an explicit re-read of the BASE config after the callback has been driven, cell 10 catches nothing at all.** (`reference_a_nc_that_leaves_your_control_green`: the isolating assertion is the whole cell.) ⚠️ **Cell 10 is also a live DATA RACE in production** — a handshake goroutine writing the shared base `*stdtls.Config` — which is a second, independent reason the clone is mandatory.

Cells 2 and 9 fired cleanly: cell 2 ⇒ `negotiated ALPN = "", want "http/1.1"` (the §5.1 control catches over-firing; ⚠️ anchor note — the SPEC calls that arm `:149-159`, the `Errorf` is at **`:158`**); cell 9 ⇒ `no-ALPN chain: GetConfigForClient is NON-nil, want nil`.

### ⚠️ 0.10 — A FIXTURE ARM THAT FAILS ITS DRIVE MAKES `AssertStats` DEAD CODE

Landing arm (vi) naively makes the subject-side drive return an error, and the runner's `t.Fatalf("subj drive: %v", err)` then aborts the subtest **before step 10 runs at all** — so the cross-side stat assertion the whole arm exists to make never executes, and the fixture fails for the wrong reason with the diagnostic the row needs suppressed. The measurement above only produced a per-arm table because arm (vi) was made **TOLERANT** (outcome logged, never appended to `probs`). `reference_fatalf_makes_assertions_unreachable`, at the FIXTURE layer rather than the unit layer. **The IMPL must make arm (vi) record its outcome rather than fail its drive**, or `AssertStats` is unreachable on exactly the side under test.

⚠️ **AND THE ANCHOR FOR THIS IS NOT UNIQUE.** The measurement agent cited `runner_test.go:1281`; re-verified at this tip the literal `t.Fatalf("subj drive: %v", err)` occurs at **`:1277` AND `:2078`** — two different runner paths — with the reference-side partner at **`:1251`**. A `sed`-by-line or a bare literal match hits the wrong one or both. **The PLAN anchors this on the enclosing runner function, never on the string or the line.** (`next-prompt.txt` §3: pathspec-scope every symbol assertion — the same hazard, one level finer, and the second instance this stage found after the seven-returns case.)

**Re-verified fixture anchors at this tip** (the agent's other cites all hold): pins at `driver/driver.go:89-90` (`wantConnectionError = 3`, `wantHandshake = 1`), their assertion sites at `:481-492`, the arm table at `:78-83` under the `:73` warning, the blank import at `runner_test.go:147`.

### 0.11 — SPEC §12 CELL 8 IS UNDER-STATED, AND §5.3 NEEDS A HELPER THAT DOES NOT EXIST

- **Cell 8** says arm (a) is RED at the tip, *"proving (b) is not satisfiable by omission."* Measured: **BOTH arms are RED at the tip**, and arm (b) is red **for the discriminating reason** — `no application protocol` fires exactly where the cert error must. So once the which-error assertion is present, arm (b) is *itself* not satisfiable by omission, and cell 8's job is narrower than claimed. ⚠️ **But the strong form is what buys this:** a weakly-written arm (b) asserting only *"not served"* would be GREEN at the tip and RED under the bug — a usable NC, yet satisfiable by omission. **The which-error triple is what makes arm (b) red at the tip too.**
- **Neither existing helper covers §5.3.** `mkDownstreamTSMutualTLS` (`manager_test.go:4485`) carries mTLS and no ALPN; `mkDownstreamTSInlineALPN` (`tls_handshake_negative_test.go:88`) carries ALPN and no mTLS. **The row owes a new `mkDownstreamTSMutualTLSALPN` plus a `startMutualTLSALPNListener`** — two helpers the SPEC's file map does not mention. All arms ran over a real started listener on **port 0**; `net.Pipe` was correctly avoided (`reference_netpipe_deadlocks_client_cert_handshake`).

### ⚠️ 0.12 — `SPEC.md` §10 FORECASTS `STATE_HISTORY.md` **+1**; IT LANDED **+2**, AND THE PRECEDENT THAT SAYS SO WAS ITS OWN IMMEDIATE PREDECESSOR

§10's row reads *"`STATE.md` / `STATE_HISTORY.md` lines | **65 / 548** | rolled in place / **+1**."* The SPEC commit's own `--numstat`:

```
30	0	docs/envoy-go/DECISIONS.md
10	10	docs/envoy-go/STATE.md
2	0	docs/envoy-go/STATE_HISTORY.md      <-- +2, NOT +1
516	0	docs/envoy-go/phases/95-tls-alpn-mismatch-fallback/SPEC.md
79	76	next-prompt.txt
```

`git show <c>:docs/envoy-go/STATE_HISTORY.md | wc -l` reads **546** at `7964f620`, **548** at `abc309d7`, **550** at `9ed6a620` — so the BRAINSTORM close ALSO moved it by **+2**. **The SPEC's `+1` contradicted the immediately preceding precedent it could have measured in one command.** The archive entry lands as a **blank line plus the entry line**; the "ONE INLINE LINE" phrasing every router uses describes the ENTRY, not the DIFF.

⚠️ **The guard triple is unaffected** — strict/parenthetical/loose went `163 / 61 / 224` -> `163 / 62 / 225` exactly as predicted, and `163 + 62 = 225` reconciles. **Only the raw line count was wrong.** ⇒ **This PLAN forecasts `550 -> 552`, and the strict guard STAYS 163 (DELTA 0).** `reference_a_stage_s_file_scope_is_measurable` again: the diff is measurable and was not measured.

### ⚠️ 0.13 — THE DATE TIE IS **GONE**, AND §0.10's "NAIVE READS 6" IS NOW STALE — BOTH EVICTION INSTRUMENTS MUST BE RE-READ, NOT INHERITED

**The tie has moved in each of the last three closes**, which is precisely why `next-prompt.txt` method note 26 says to read rather than predict:

| tip | §Recent dates, newest -> oldest | discriminator |
|---|---|---|
| `7964f620` (94 IMPL) | `09-03, 09-02, 09-01, 09-01, 08-31` | tie in the MIDDLE; unique oldest at the tail |
| `abc309d7` (router) | `09-05, 09-03, 09-02, 09-01, 09-01` | **tie AT the oldest position — a date read CANNOT discriminate** |
| **`9ed6a620` (this tip)** | **`09-06, 09-05, 09-03, 09-02, 09-01`** | **NO TIE — five distinct dates** |

⇒ At this tip the date read and the list position **agree**, and both name the same evictee: **`phase 94 (tls-connection-error-stat) BRAINSTORM done`** (`2026-09-01`, and the tail). §13.3's post-close prediction is CONFIRMED exactly.

**And §0.10's bare-form figures have already rotted.** §0.10 measured, at `abc309d7`, strict **5** / naive **6**, the sixth naive hit being the `last-updated` bullet at `:20` whose prose quoted the phrase. **At this tip the naive form reads 5**, because the SPEC's own roll (`STATE.md` `10 10`) rewrote that bullet and the phrase is gone.

| form on `STATE.md` | at `abc309d7` | **at this tip** |
|---|---|---|
| strict `^- \*\*prior active-phase:\*\* ` | 5 | **5** |
| naive substring `prior active-phase` | **6** | **5** |

⚠️ **Neither bare form answers the eviction question in EITHER state** — both count all five §Recent entries and are **invariant under which entry is evicted** (5 in, 5 out). Only the **LABEL-BOUND PAIR** discriminates, and it must be read on BOTH files:

| half | at this tip | must read after this close |
|---|---|---|
| `grep -cF -- "<evictee label>" docs/envoy-go/STATE.md` | **1** | **0** |
| `grep -cF -- "<evictee label>" docs/envoy-go/STATE_HISTORY.md` | **0** | **1** |

**The instrument was proven live in both directions, not assumed:** the PREVIOUS evictee `phase 93 (h2-local-reply-content-length) IMPL done` reads `STATE.md` **0** / `STATE_HISTORY.md` **1** at this tip and read **1** / **0** at `abc309d7` — **the pair DID swap across the SPEC commit.** ⚠️ Pass `--` before any `grep -F` pattern that could begin with `-`.

### ⚠️ 0.14 — `grep` IS A DIFFERENT PROGRAM IN A SUBAGENT SHELL THAN IN THE CONTROLLER SHELL, AND `next-prompt.txt`'s BLINDNESS NOTE IS TRUE FOR ONLY ONE OF THEM

`next-prompt.txt` carries a whole section — *"A RECURSIVE `grep` IN THIS HARNESS IS BLIND TO `next-prompt.txt`"* — asserting that `grep` is a shell **function** (ugrep 7.8.4, `--ignore-files`, honouring `.gitignore`). A measurement agent this session reported the opposite: `type grep` printed `grep is /usr/bin/grep`, GNU grep 3.11, no function.

**Both are correct, in their own shell.** Measured directly in the controller shell at this tip:

| shell | `type grep` | `grep --version` | recursive sweep sees `next-prompt.txt`? |
|---|---|---|---|
| **controller** | `grep is a function` | **ugrep 7.8.4** | **NO** — `grep -rl 'AUTONOMOUS LOOP CONTROL' … --include='*.txt'` prints NOTHING |
| **controller, `command grep`** | — | GNU 3.11 | **YES** — prints `/home/esa/git/envoy-go/next-prompt.txt` |
| **subagent** | `grep is /usr/bin/grep` | GNU 3.11 | **YES**, with the bare name |

⚠️ **THE CONSEQUENCE IS A SILENT CROSS-AGENT DISAGREEMENT.** A subagent handed the router's blindness warning is being told to defend against a hazard its own shell does not have; worse, a controller that *reproduces* a subagent's bare-`grep` command gets a DIFFERENT answer on any recursive, `.gitignore`-sensitive sweep — and neither party is wrong. `next-prompt.txt` §2's rule — *"when two agents contradict each other, FIND THE VARIABLE rather than picking a winner"* — applies, and **the variable is the shell, not the tree.**

**What was checked before carrying any figure forward.** Every anchored-occurrence count this stage quotes was run under BOTH tools and they AGREE exactly:

| figure | ugrep | GNU |
|---|---|---|
| `-family row` occurrences | 96 | 96 |
| archive strict `^- \*\*prior active-phase:\*\* ` | 163 | 163 |
| archive parenthetical `^- \*\*prior active-phase \(` | 62 | 62 |
| archive loose `^- \*\*prior active-phase` | 225 | 225 |

⇒ **The divergence is confined to recursive, `.gitignore`-honouring sweeps**, which is exactly the class the router's note describes. The note is not wrong; it is **under-scoped**. **Corrected form: name the SHELL as well as the TOOL.** In the controller shell use `command grep -rl`, `git grep`, or a direct path for anything that must see `next-prompt.txt`; in a subagent shell the bare name already works, and `command grep` does NOT survive `xargs` in either.

### 0.15 — THE TIP BASELINE, AND THE PATCH'S BLAST RADIUS, BOTH MEASURED AS A PACKAGE SWEEP

SPEC §0.8: the BRAINSTORM's 11-test figure came from a `./internal/listener/` selector while the production edit is in `internal/tls`, whose three `NextProtos` assertions were *"expected, not measured."*

`go test ./internal/tls/ ./internal/listener/ -count=1 -v` at the tip ⇒ **`RC=0`, `RUN=326`, anchored FAIL 0, `[no tests to run]` 0** (`internal/tls` 178 + `internal/listener` 148 = 326, reconciled).

- **§0.8 DISCHARGED:** the three sites are `TestNewDownstreamConfig_Happy`, `TestNewUpstreamConfig_Happy`, `TestNewQUICDownstreamConfig_ALPNh3`. All three **PASS at the tip AND under the patch.** ⚠️ Anchor correction: the SPEC cites `config_test.go:1193-1204`; `:1193` is the last line of the DOC COMMENT and the assertion is **`:1203-1204`**.
- **§0.9 CONFIRMED:** `_ALPNNegotiationFailure_Aborts` passes at the tip while pinning the wrong side, logging `handshake: tls: client requested unsupported application protocols (["bogus/9"])`.
- **Under the §4 patch, applied verbatim** (`gofmt -l` empty; `config.go` 582 -> **629** lines; `installALPNMismatchFallback` at `:602`, `alpnOverlaps` at `:620`; the call at `:57`, inside `NewDownstreamConfig` and before every return): `internal/tls` stays `RC=0`/178/0. `internal/listener` goes `RC=1`, and **EXACTLY ONE test fails** — `_ALPNNegotiationFailure_Aborts` at `tls_handshake_negative_test.go:169`. **That is §12 NC 1 firing as designed, a real green->red transition on the wrong-side pin, with no other landed test in either package disturbed.**

### 0.16 — EVERY §0.3 / §0.4 ANCHOR CORRECTION IS CONFIRMED; NOTHING IN THEM IS REFUTED

Independently re-read at `9ed6a620`: the six returns **128 195 220 229 248 258** (the inherited `129 196 221 230 249 259` set is wrong); `func NewDownstreamConfig` **:42**; the builder call **:53** with its error block closing at **:56**, so the install is the new **:57**; `cfg.ClientCAs`/`cfg.ClientAuth` at **94**/**95**, both AFTER the anchor — **§0.1's bypass argument holds structurally**; QUIC builder **:268**, its return **:290** (not `:291`); the `NextProtos` append **:579**; `_Aborts` **121-171** with its doc comment **113-120**; the helper doc **85-87** with `mkDownstreamTSInlineALPN` at **:88**; `alpnMatchAny` at `chainmatch.go:293`; `BEHAVIOR_CONTRACT.md` **5989** lines with `:1944` **357** chars, `:1961` **736**, `:1971` **5098**. **Two independent readers agree on all of it.**

### ⚠️ 0.17 — THE ONE SURVIVING `no_application_protocol` SITE IS A NON-SITE THAT NEEDS AN EXPLICIT GUARD

Enumerated over `internal/*`, `test/*` and `BEHAVIOR_CONTRACT.md`, `no_application_protocol` occurs at exactly **four** lines, three of which §7 already owns (`tls_handshake_negative_test.go:117`, `:118`, `:169` — all inside the `:113-171` rewrite). The fourth is **`internal/listener/quic_negative_test.go:92`**:

> *"a client offering only ALPN h2 must fail the QUIC/TLS handshake (reference Envoy's QUIC listener is h3-only; crypto/tls surfaces `no_application_protocol`)"*

**It STAYS TRUE and must NOT be touched** — §2.2 shows `negotiateALPN`'s RFC 9001 §8.1 guard is itself predicated on `len(serverProtos) != 0`, so the TCP-only install leaves QUIC enforcement intact, and touching QUIC would silently REMOVE a mandated rejection. But after the row it is the **ONLY** surviving `no_application_protocol` mention in the tree, which makes it the exact thing a `grep`-driven cleanup sweep deletes last and by accident. **The PLAN names it as a GUARDED NON-SITE and the IMPL asserts it is byte-untouched.**

---

## 1. Stage scope, MEASURED

### 1.1 What this PLAN commit touches

`docs/envoy-go/STATE.md` · `docs/envoy-go/STATE_HISTORY.md` · `docs/envoy-go/phases/95-tls-alpn-mismatch-fallback/PLAN.md` · `next-prompt.txt`. **Nothing else.**

### 1.2 Why — MEASURED across FOUR precedent PLAN commits at this tip

`next-prompt.txt` method note 22 names ONE precedent (`db539e7d`) and tells this stage to measure against it. Method note 22 also records that its own predecessor was REFUTED for under-enumerating, so one precedent is not enough. Four were measured with `git show --numstat`:

| precedent | files touched |
|---|---|
| `db539e7d` (94 PLAN) | `STATE.md` (9/9) · `STATE_HISTORY.md` (+2/-0) · `phases/94/PLAN.md` (+1662/-0) · `next-prompt.txt` (93/106) |
| `90010c4c` (93 PLAN) | `STATE.md` (9/8) · `STATE_HISTORY.md` (+2/-0) · `phases/93/PLAN.md` (+1031/-0) · `next-prompt.txt` (76/65) |
| `bae5e24d` (75 PLAN) | `STATE.md` (10/10) · `phases/75/PLAN.md` (+1394/-0) · `phases/75/PROGRESS.md` (+115/-0) · `next-prompt.txt` (46/45) |
| `acedfd2b` (77 PLAN) | `STATE.md` (6/6) · `phases/77/PLAN.md` (+2212/-0) · `phases/77/PROGRESS.md` (+97/-0) · `next-prompt.txt` (77/49) |

- **ALWAYS, all four:** `STATE.md`, `next-prompt.txt`, the phase's own `PLAN.md`.
- **NEVER, all four:** ⚠️ **`DECISIONS.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, and every file under `internal/` and `test/`.** ⇒ `ADR-0317` STAYS `PROPOSED`, the house guard STAYS **ARMED**, tail STAYS `ADR-0317`, next-free STAYS `ADR-0318`, and the `SPEC.md` §11 contract map STAYS PINNED. **The IMPL lands all of it.**
- **CONDITIONAL, each evaluated at THIS tip rather than inherited:**
  - `STATE_HISTORY.md` — appended at the 93 and 94 PLANs, not at 75/77 (which predate the ADR-0288 archive). Driven by whether §Recent is at the five-entry cap. **It IS at cap** ⇒ this PLAN owes an eviction and a parenthetical append.
  - `PROGRESS.md` — created at the 75 and 77 PLANs, **NOT** at 93 or 94. **Phase 95 follows the 93/94 posture: no `PROGRESS.md` at the PLAN.** `STATE.md` continues to record *"No `PROGRESS.md`, no `REVIEW.md`"* as a STANDING DEPARTURE, **named, not claimed.**

### 1.3 The split gate — EVALUATED, NOT SPLIT, AND THE REASONING IS STATED SO A REVIEWER CAN OVERTURN IT

`BOOTSTRAP_PROMPT.md` §6.1 (verified at the repo root at this tip, `:285-290`) triggers a split if `PLAN.md` exceeds **~25 numbered tasks** OR estimates exceed **~1500 lines of code** of net change.

**Task count: 18** (§5). **DERIVED here, not carried** — `SPEC.md` §15 item 1 deliberately quotes no figure. Under the gate.

**LoC — anchored on MEASURED prototypes wherever one exists, and labelled where it does not:**

| component | basis | estimate |
|---|---|---|
| `internal/tls/config.go` | **MEASURED**: the §4 patch applied verbatim took the file 582 -> **629** | **+47 / -0** |
| `internal/tls` guard tests (§5.4) | measured prototype (`zzm2guard_test.go`, three tests + the post-drive base re-read) | **+150** |
| `internal/listener/tls_handshake_negative_test.go` (§5.1 + §0.5 comments) | rewrite of `113-171` plus two doc blocks | **+70 / -35** |
| `internal/listener/manager_test.go` rows 7/8 + `wantLeaves` + `startOneWayTLSListenerALPN` | measured probe | **+180 / -25** |
| `internal/listener/manager_test.go` §5.3 + the TWO missing helpers (§0.11) | measured prototype (4 arms, mTLS+ALPN listener) | **+190** |
| fixture `0120` driver: arm (vi) + the §0.2 overlap arm + re-measured pins | measured instrumentation | **+130 / -12** |
| fixture `0120` YAMLs (both sides) | one key each | **+2 / -0** |
| fixture `0120` `expectations.yaml` + `README.md` | six-arm tables + pin blocks | **+45 / -20** |
| production + test comment reconciliation (§0.5, §0.17, §7) | 4 production sites + 2 false present-tense sites + 9 guarded non-sites | **+45 / -20** |
| docs: `ADR-0317` §Decision/§Consequences, `BEHAVIOR_CONTRACT.md:1944`, `ROADMAP.md`, `STATE.md`, `PROGRESS.md` | precedent | **+130 / -15** |

**Total ≈ +989 / -127.** Counting only `.go` files: `47 + 150 + 70 + 180 + 190 + 130 + 45` ≈ **+812**.

**DECISION: DO NOT SPLIT.** Three grounds, in order of weight:

1. **Both thresholds are clear on the narrow AND the broad reading.** 18 tasks against ~25; ~**+812** `.go` lines, or ~**+989** counting YAML and Markdown, against ~1500. Unlike phase 94 — which crossed the broad reading by ~7% and split anyway on argument — this row is not close to either limit.
2. **The single production edit is 47 measured lines.** The bulk of the cost is test and fixture surface whose whole purpose is to make those 47 lines falsifiable. Splitting the pins away from the code is the one split that would defeat the row.
3. ⚠️ **The only clean seam — {unit layer} / {fixture `0120`} — would ship the security fix of §0.1 with no cross-side evidence.** §0.1 is an authentication bypass CONFIRMED BY EXECUTION (§0.1 above): the arm proving mandatory mTLS survives the fallback is a unit arm, but the arm proving the reference actually SERVES a mismatched connection is the fixture. Shipping either alone ships an unreconciled security-relevant change.

⚠️ **`BOOTSTRAP_PROMPT.md` §6.1's mid-execution trigger still applies.** If any single task's sub-steps blow past ~10 items, the split happens then. The likeliest candidates are **Task 7** (§5.3, four arms plus two new helpers) and **Task 13** (the §0.2 cell-13 replacement).

---

## 2. Sentinel — RUN MECHANICALLY AT `9ed6a620`, ACTUAL OUTPUT

⚠️ **A PLAN moves none of this. That is PROVEN below, not assumed** — the checks were run at this stage's own tip before any edit, and `next-prompt.txt` method note 1 forbids inheriting an NC shape across a stage.

- **(1)** `want=127`, field-parsed form verbatim: **ONE** line — `NOT DONE: row 95`.
- **(2)** **SIX** hits, at `:205 :211 :217 :227 :233 :241`.
- **(3)** **SILENT.**

⇒ **The sentinel does NOT fire. `stop` was evaluated and deliberately NOT created** — verified absent at the git root and in every stage worktree.

Per-line md5 of the six check-(2) windows, **trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`) — all six byte-identical to the phase-95 SPEC close:
`205 10d7807bf02d` · `211 4a92f7e62fc6` · `217 2a7eb298b9fd` · `227 242e53c6f7a3` · `233 b2680e6f4fbf` · `241 6caa1c3ce0e7`
⚠️ **The digest is METHOD-SENSITIVE** — these match ONLY with the trailing newline included. State the method whenever the digest is quoted.

### 2.1 The four NCs and the check-(2) positive control — ALL RUN, ALL FIRED

- **NC-A** (doctor row 62 to `in-progress`, `want=127`, on a scratch copy): `NC LANDED? [ in-progress ]` INSPECTED FIRST; check (1) then reads **TWO** lines — `NOT DONE: row 62`, `NOT DONE: row 95`.
- **NC-B** (the denominator, `want=126`, on the REAL file): **TWO** lines — `NOT DONE: row 95`, then `GATE FAIL: examined 127 data rows, expected 126`.
- **NC-C** (the MANDATORY check-(3) NC, because check (3) is silent and a silent check is indistinguishable from a broken one): residual **0**, and `NEVER OPENED: gRPC` **FIRED**.
- **NC-D** (`-family row`, with `--` before the pattern): **96** occurrences / **68** lines. ⚠️ Without `--` the leading `-` reads as an OPTION, `grep -c` fails, and the surrounding arithmetic prints `0` — **which reads exactly like "no change."**
- **Check-(2) positive control**: residual **0** with **6** substitutions ASSERTED. ⚠️ BOTH phrases must be substituted: the longer does not contain the shorter as a substring, so a one-phrase control reports a residual of 5 that reads like a finding.

---

## 3. The design record, as CORRECTED by this stage

`SPEC.md` §2-§4 stand unchanged on the MECHANISM. Three things about the surrounding pins change, and one thing about the design is now proven rather than argued.

### 3.1 The production edit — VERBATIM, and MEASURED to compile and pass

`SPEC.md` §4's code was applied byte-for-byte in a throwaway worktree. It **compiles, `gofmt -l` prints nothing, requires NO new import**, and takes `internal/tls/config.go` from **582 to 629** lines. `installALPNMismatchFallback` lands at `:602`, `alpnOverlaps` at `:620`, and the call `installALPNMismatchFallback(cfg)` at the new **`:57`** — inside `NewDownstreamConfig` (post-patch span `42-260`) and before the first return (post-patch `:129`). **This is the code Task 2 lands. It is not re-opened.**

### 3.2 What is now PROVEN rather than reasoned

- **The per-handshake `Clone()` is mandatory, and the alternative is an authentication bypass that SERVES traffic** (§0.1). The SPEC argued this from mutation order; it is now instrumented at both instants and driven to a full application round trip. **Two independent reasons now require the clone**: the `:94-95` ordering, and the data race §0.9 exposes — a build-time snapshot's in-place counterpart writes the shared base `*stdtls.Config` from a handshake goroutine.
- **The TCP-only install is correct and the QUIC path is genuinely untouched** — and §0.6 shows exactly how an IMPL could get this wrong: an unscoped grep for the return statement reads **seven**, and the seventh is the QUIC return.
- **The predicate's deliberate subsumption of `http11fallback` is real and measured** (§0.8): the mismatch branch is entered for row 8 as well as row 7.

### 3.3 What the SPEC got right and this stage confirms

Every anchor correction in `SPEC.md` §0.3/§0.4 is confirmed by two independent readers (§0.16). §6.2's arm arithmetic prediction — `wantConnectionError` STAYS **3**, `wantHandshake` **1 -> 2** — is confirmed by a booted cross-side measurement (§0.7). §15 item 4's prediction that row 7 is RED and row 8 GREEN at the tip is confirmed (§0.4). §12 cell 11 is confirmed. §13.3's post-close §Recent shape is confirmed exactly (§0.13).

---

## 4. File structure

**Production (3 files):**

| file | change |
|---|---|
| `internal/tls/config.go` | `installALPNMismatchFallback` + `alpnOverlaps` + the call at the new `:57`; the `:25` doc-comment correction; the `:264` QUIC doc clause |
| `internal/tls/doc.go` | the `:5` doc-comment correction |
| `internal/listener/manager.go` | `:138` — APPEND to the history block, do NOT revert it |

**Tests (3 files):** `internal/tls/config_test.go` (§5.4 guards) · `internal/listener/tls_handshake_negative_test.go` (§5.1 flip, the `:85-87` helper doc, **and the `:27` false present-tense claim of §0.5**) · `internal/listener/manager_test.go` (§5.2 rows 7/8 + `wantLeaves`, §5.3 + two new helpers, **and the `:1086` false present-tense claim of §0.5**).

**Fixture `test/fixtures/0120-tls-connection-error/` (5 files):** `driver/driver.go` · `envoy.yaml` · `envoy-go.yaml` · `expectations.yaml` · `README.md`. ⚠️ **No new fixture directory, so the three registration gates are untouched and `runner_test.go` is BYTE-UNTOUCHED** — `0120` is EXTENDED, `0121` stays free, fixtures stay **122**.

**Guarded NON-sites — asserted byte-untouched at Task 18 (§0.5, §0.17):** `internal/bootstrap/bootstrap.go:84` · `internal/listener/manager_test.go:1023, :1030, :1276, :1732` · `test/fixtures/0002-tls-tcp/driver/driver.go:196, :248` · `test/fixtures/0002-tls-tcp/envoy-go.yaml:14` · **`internal/listener/quic_negative_test.go:92`**.

**Docs at the IMPL:** `DECISIONS.md` (ADR-0317 §Decision + §Consequences, and DISARM the house guard) · `BEHAVIOR_CONTRACT.md:1944` · `ROADMAP.md` (row 95 -> `done`) · `STATE.md` · `PROGRESS.md`.

### 4.1 STABLE ANCHORS — use these, not line numbers

Line anchors drift, and this stage found two that are **not unique** (§0.6, §0.10). Every task anchors on literal text or an enclosing symbol.

| target | STABLE anchor |
|---|---|
| the install point | the `commonTLSContextToConfig(ctx.GetCommonTlsContext(), …, "downstream", provider)` call and the `}` closing its `if err != nil` block |
| ⚠️ the six TCP returns | **the enclosing `func NewDownstreamConfig(` … NEVER the return text** — unscoped it reads **7** and includes the QUIC return (§0.6) |
| ⚠️ the QUIC builder (untouched) | `func NewQUICDownstreamConfig(` |
| the client-auth assignment | `cfg.ClientAuth = clientAuthFor(require)` |
| the `NextProtos` append | `cfg.NextProtos = append(cfg.NextProtos, c.GetAlpnProtocols()...)` |
| the flipped test | `func TestNewManager_LiveHandshake_ALPNNegotiationFailure_Aborts(` (rename target) |
| its control arm | `negotiated ALPN = %q, want %q` with `"http/1.1"` |
| the ALPN helper | `func mkDownstreamTSInlineALPN(` |
| the one-way listener helper | `func startOneWayTLSListener(` |
| the mTLS helper | `func mkDownstreamTSMutualTLS(` |
| the counter table | `func TestServeConnection_SSLConnectionErrorCounter(` |
| the "no other ssl.\* leaf moved" loop | the `sslLeafRoster` range inside that function |
| fixture pins | `wantConnectionError =` / `wantHandshake =` |
| fixture arm table | `⚠️ THESE ARE ARM ARITHMETIC.` |
| ⚠️ the runner's subject-drive fatal | **the enclosing runner function — the literal `t.Fatalf("subj drive: %v", err)` occurs TWICE (`:1277`, `:2078`), with the reference partner at `:1251`** (§0.10) |
| the contract clause | `is not surfaced to the fixture driver in phase 03` (occurs **exactly once**, verified) |
| the house ADR guard | `^> \*\*STATUS: PROPOSED` resolved BACKWARD to `## ADR-0317` |

⚠️ **PATHSPEC-SCOPE every `sed`**, and **RANGE-SCOPE anything inside `config.go`**.

---

## 5. Tasks

**18 tasks.** The count is DERIVED at this stage (§1.3); `SPEC.md` §15 item 1 deliberately carries no figure. Ordering is TDD-first within each task and dependency-ordered across them: the guard pins (T1) are RED before the code lands (T2); the flip (T4) is a green->red->green transition on a landed test; the fixture pins (T12) do not land before the arm that moves them (T11).

⚠️ **Every task ends with a commit.** Subagents commit LOCALLY on their own stage branch with EXPLICIT PATHSPECS; the controller merges and squashes. ⚠️ **Subagents do not push.**

---

### Task 1: §5.4 guard pins in `internal/tls` — RED FIRST

**Files:** Test `internal/tls/config_test.go`
**Interfaces:** Produces the three guard assertions T2 turns green.

- [ ] **Step 1.** Build the contexts with the package's OWN helpers (`makeTransportSocket` / `inlineBytes` / the `pki` helper) — ⚠️ **NOT a hand-rolled `*stdtls.Config`** (`next-prompt.txt` §7h: a table can carry an invented input no production path produces).
- [ ] **Step 2.** Three assertions:
  - a context WITHOUT `alpn_protocols` ⇒ `dc.TLSConfig.GetConfigForClient == nil`;
  - a context WITH `alpn_protocols` ⇒ `dc.TLSConfig.GetConfigForClient != nil` **and** `dc.TLSConfig.NextProtos` unchanged;
  - `require_client_certificate: true` + `alpn_protocols` ⇒ the config's `ClientAuth`/`ClientCAs` are the completed values.
- [ ] **Step 3. RUN THEM RED.** At the tip assertion 2 must fail with a message naming the symbol — the measured prototype produced `ALPN chain: GetConfigForClient is nil — the fallback is NOT installed`. ⚠️ **`t.Errorf` per property, `t.Helper()` on shared assertions** so each failure gets a distinct call-site line.
- [ ] **Step 4.** `go test ./internal/tls/ -count=1 -v -run '<selector>'` — assert `RUN` nonzero and that `[no tests to run]` does NOT appear.

**Commit.**

---

### Task 2: The production edit — `internal/tls/config.go`, and PROVE IT LANDED

**Files:** Modify `internal/tls/config.go`
**Interfaces:** Consumes nothing. Produces `installALPNMismatchFallback`, `alpnOverlaps`. Turns T1 green.

- [ ] **Step 1.** Apply `SPEC.md` §4 VERBATIM — the two unexported functions plus the single call. **It is measured to compile and pass and to need no new import** (§3.1). Do not re-derive it.
- [ ] **Step 2. INSTALL AT THE ENTRY.** Anchor on the `commonTLSContextToConfig` call and its error block (§4.1). ⚠️ **"At the return" is not a placement** — a prototype placed before the FIRST return landed in the SDS validate-mode arm, built, booted, served, and changed nothing.
- [ ] **Step 3. ⚠️ DO NOT TOUCH `NewQUICDownstreamConfig`.** §2.2: `negotiateALPN`'s RFC 9001 §8.1 rejection is itself predicated on `len(serverProtos) != 0`, so emptying `NextProtos` there would make a MANDATED rejection VACUOUS. ⚠️ **An unscoped grep for the return statement reads SEVEN and the seventh is that function's return** (§0.6).
- [ ] **Step 4. ASSERT THE SYMBOL, THEN DRIVE THE ARM.** A build is not evidence. Assert (a) `installALPNMismatchFallback(cfg)` occurs inside `func NewDownstreamConfig(` and before its first return, and (b) T1 flips to green. Expected shape, measured: file 582 -> **629**.
- [ ] **Step 5. `panic()` REACHABILITY CONTROL** (`SPEC.md` §15 item 6). Insert `panic(...)` immediately above `alt := cfg.Clone()`, drive four offers, and record which enter the branch. **Measured expectation: `["bogus/9"]` vs `["h2","http/1.1"]` PANICS; `["http/1.1"]` vs `["h2"]` PANICS; an overlapping offer does NOT; an absent offer does NOT.** ⚠️ There is no `recover()` in non-test `internal/tls` or `internal/listener`, so the panic aborts the binary — gate on `grep -cE '^panic:'`. **Remove the panic and re-run before committing.**
- [ ] **Step 6.** `gofmt -l` (gate on OUTPUT) and the US-locale misspell sweep on the new comments.

**Commit.**

---

### Task 3: §5.4's third bullet — the callback driven DIRECTLY, with the base re-read that makes NC 10 visible

**Files:** Test `internal/tls/config_test.go`
**Interfaces:** Consumes T2.

- [ ] **Step 1.** Invoke the callback directly with a `*stdtls.ClientHelloInfo` for each of: empty `SupportedProtos` ⇒ `(nil, nil)`; overlapping ⇒ `(nil, nil)`; non-overlapping ⇒ non-nil with `NextProtos == nil` and `GetConfigForClient == nil`.
- [ ] **Step 2.** On the non-overlapping arm assert `ClientAuth` and `ClientCAs` EQUAL the base config's — the direct unit expression of the §0.1 bypass. Measured green shape: `ALT: ClientAuth=RequireAndVerifyClientCert ClientCAs==base? true NextProtos=nil callback==nil? true`.
- [ ] **Step 3. ⚠️ MANDATORY, AND THE WHOLE POINT OF THE TASK: RE-READ THE BASE CONFIG *AFTER* DRIVING THE CALLBACK** and assert its `NextProtos` is still the advertised list. **§0.9 measured that without this assertion NC 10 is completely invisible** — `alt.NextProtos == nil`, `alt.GetConfigForClient == nil`, `alt.ClientAuth` and `alt.ClientCAs` all stay GREEN under the in-place-mutation bug, because `Clone()` faithfully copies an already-emptied slice.
- [ ] **Step 4. NC it (cell 10):** mutate `cfg.NextProtos` in place before cloning; **only the Step-3 assertion may fire.** Expected message shape: `base NextProtos AFTER callback = [], want [h2 http/1.1] — the base config was mutated`. Revert.
- [ ] **Step 5. NC it (cell 9):** remove the `len(cfg.NextProtos) == 0` guard; T1's first assertion must redden (`no-ALPN chain: GetConfigForClient is NON-nil, want nil`). Revert.

**Commit.**

---

### Task 4: §5.1 — flip the test that pins the wrong side

**Files:** Test `internal/listener/tls_handshake_negative_test.go`
**Interfaces:** Consumes T2. **This is `SPEC.md` §12 NC 1 and it is a REAL green->red->green transition, not a fresh green.**

- [ ] **Step 1.** Rename `TestNewManager_LiveHandshake_ALPNNegotiationFailure_Aborts` -> `TestNewManager_LiveHandshake_ALPNMismatch_CompletesWithNoProtocol`.
- [ ] **Step 2.** Rewrite the doc comment (currently `113-120`) — it asserts the FALSE parity *"matching reference Envoy, which alerts `no_application_protocol` on ALPN mismatch"*. The replacement states the MEASURED reference behaviour (§0.7: handshake OK, `negotiated=""`, connection **SERVED**, `connection_error +0`, `handshake +1`) and cites **ADR-0317**.
- [ ] **Step 3.** Hoist the inline `stats.NewRegistry()` into a local `reg`.
- [ ] **Step 4.** Invert the arm (currently `162-170`): the handshake must SUCCEED and `ConnectionState().NegotiatedProtocol` must equal `""` **by exact equality**.
- [ ] **Step 5.** Assert the counters after both arms drain, via `counterValue` + `awaitDrained`: `ssl.connection_error == 0`, `ssl.handshake == 2`. ⚠️ **Poll the gauge; NO SLEEPS.** ⚠️ **Never register a stat inside `Registry.Walk`** — the callback runs under `RLock` and registering re-enters the write lock, DEADLOCKING the process.
- [ ] **Step 6. KEEP the control arm and keep it FIRST** (currently `149-159`, its `Errorf` at `:158`) so the `handshake == 2` arithmetic is unambiguous. It asserts `NegotiatedProtocol == "http/1.1"` by exact equality and is the OVER-firing guard.
- [ ] **Step 7. `t.Errorf` per property**, except the setup failures which stay fatal (`reference_fatalf_makes_assertions_unreachable`).
- [ ] **Step 8. NC it (cell 2):** drop the overlap check so the callback fires unconditionally; **Step 6's control must redden** — measured: `negotiated ALPN = "", want "http/1.1"`. Revert.
- [ ] **Step 9.** Confirm the blast radius: `go test ./internal/listener/ -count=1 -v` — **exactly this one test transitions**, and under the patch no other landed test in either package is disturbed (§0.15).

**Commit.**

---

### Task 5: The comment sites §7 MISSED — two FALSE claims and nine guarded non-sites

**Files:** Test `internal/listener/tls_handshake_negative_test.go`, `internal/listener/manager_test.go`
**Interfaces:** None. **This task exists because of §0.5 and it is not cosmetic.**

- [ ] **Step 1.** Rewrite the helper doc at `85-87` — *"the server ENFORCES ALPN overlap"* (`SPEC.md` §0.6).
- [ ] **Step 2. ⚠️ Rewrite `tls_handshake_negative_test.go:27`**, which claims envoy-go aborts *"via SelectChain returning an error from the `GetConfigForClient` callback."* **This is ALREADY FALSE at the tip** — that callback was deleted at phase 07.2 Task 10 — and this row makes it actively misleading by installing the first real `GetConfigForClient` since phase 03, one that does ALPN fallback and **has no error path at all** (§2.3).
- [ ] **Step 3. ⚠️ Rewrite `manager_test.go:1086`** — *"route correctly via `GetConfigForClient`"* — false for the same reason and refuted by its own file at `:1030`.
- [ ] **Step 4. GUARD, DO NOT EDIT,** the nine history-class sites of §0.5 and the `no_application_protocol` non-site of §0.17. They are factually correct AS HISTORY. **Task 18 asserts them byte-untouched.**
- [ ] **Step 5.** ⚠️ **Audit case-INSENSITIVELY** (`grep -i`) — a case-sensitive grep is blind to uppercase prose — and **PATHSPEC-SCOPE** the sweep.

**Commit.**

---

### Task 6: §5.2 rows 7 and 8, and the `wantLeaves` map the SPEC's figure would have broken

**Files:** Test `internal/listener/manager_test.go`
**Interfaces:** Consumes T2. Produces `startOneWayTLSListenerALPN`.

- [ ] **Step 1.** Add `startOneWayTLSListenerALPN(t, pki, alpn []string) (*stats.Registry, string)` beside `startOneWayTLSListener`, identical except it calls `mkDownstreamTSInlineALPN`. Same package; no export, no move.
- [ ] **Step 2.** Add the `listen` field to the row struct, defaulting to `startOneWayTLSListener` when nil, so the six existing rows are untouched. ⚠️ **If a second table function proves cleaner, the "no other `ssl.*` leaf moved" loop must be DUPLICATED, not dropped.**
- [ ] **Step 3.** Add the two rows:

| # | name | listener ALPN | client offer | `connection_error` | `handshake` | **`no_certificate`** |
|---|---|---|---|---|---|---|
| 7 | `alpn_mismatch_bogus` | `["h2","http/1.1"]` | `["bogus/9"]` | 0 | 1 | **1** |
| 8 (control) | `alpn_http11_vs_h2_only` | `["h2"]` | `["http/1.1"]` | 0 | 1 | **1** |

- [ ] **Step 4. ⚠️ `wantLeaves` IS `{handshake: 1, no_certificate: 1}`, NOT `{handshake: 1}`.** §0.4: `startOneWayTLSListener` sends no `CertificateRequest`, so **every completing handshake books `ssl.no_certificate`**. **A map written from `SPEC.md` §5.2's "rest 0" goes RED against a CORRECT implementation.** The six existing rows are unaffected — none of them completes a handshake.
- [ ] **Step 5.** Merge `wantLeaves` over a zero default so every row still asserts which counters did NOT fire (`next-prompt.txt` §7g).
- [ ] **Step 6. RECORD THE PRE-FIX STATE** (`SPEC.md` §15 item 4 — already measured, §0.4): **row 7 RED at the tip, row 8 GREEN at the tip.**
- [ ] **Step 7. ⚠️ STATE ROW 8's ACTUAL SCOPE IN A COMMENT** (§0.8): post-fix the callback intercepts row 8 BEFORE the stdlib's `http11fallback`, so **post-fix row 8 is green under both a correct fix and a stdlib-only one — only row 7 discriminates.** Row 8's value is entirely PRE-FIX; the post-fix over-firing guard is Task 4's control arm.
- [ ] **Step 8. NC it (cell 6):** set one row's `wantLeaves` to the wrong leaf; it must redden. Revert.

**Commit.**

---

### Task 7: §5.3 — the mTLS-preservation arms, and the TWO helpers that do not exist

**Files:** Test `internal/listener/manager_test.go`
**Interfaces:** Consumes T2. Produces `mkDownstreamTSMutualTLSALPN`, `startMutualTLSALPNListener`. ⚠️ **Split candidate** (§1.3).

- [ ] **Step 1. Write the two helpers** (§0.11): neither `mkDownstreamTSMutualTLS` (mTLS, no ALPN) nor `mkDownstreamTSInlineALPN` (ALPN, no mTLS) covers this. ⚠️ **Use a real listener on PORT 0** — `net.Pipe` DEADLOCKS a client-cert handshake.
- [ ] **Step 2.** `TestServeConnection_ALPNMismatch_PreservesClientAuth`, one listener (`alpn_protocols: ["h2","http/1.1"]` + `require_client_certificate: true`), arms:

| arm | client cert | ALPN offer | expected |
|---|---|---|---|
| control | valid | `["http/1.1"]` | served, `negotiated == "http/1.1"` |
| a | valid | `["bogus/9"]` | **served**, `negotiated == ""`, `certSent == true` |
| b (TLS1.3) | withheld | `["bogus/9"]` | **REJECTED**, not served |
| b (TLS1.2) | withheld | `["bogus/9"]` | **REJECTED**, not served |

- [ ] **Step 3. ⚠️ DISCRIMINATE ON THE SERVER-SIDE COUNTER TRIPLE, NOT THE CLIENT ERROR STRING** (§0.3). A `strings.Contains(err, "certificate")` assertion **FAILS AGAINST THE CORRECT IMPLEMENTATION at TLS1.2**, where the client reads `remote error: tls: handshake failure` with no `certificate` substring (`crypto/tls/handshake_server.go:966-971` sends `alertCertificateRequired` only at TLS1.3). **Assert `ssl.fail_verify_no_cert == 1` AND `ssl.connection_error == 0` AND `ssl.handshake == 0`** — which error fired and which did not, version-invariantly. Any client-string assertion is scoped to TLS1.3 EXPLICITLY.
- [ ] **Step 4. ⚠️ "SERVED" MEANS A POST-HANDSHAKE APPLICATION ROUND TRIP**, not a completed handshake. `openssl s_client` answers the wrong question; drive a Go probe that sends and reads.
- [ ] **Step 5. ⚠️ FORCE-SEND arm (a)'s certificate via `GetClientCertificate` and RECORD that it fired.** A Go client silently withholds a cert whose issuer is not in the server's `certificate_authorities` (`reference_go_client_cert_withholding`), which would make the arm vacuous. **`certSent` is an assertion, not a log line** — under the §0.1 bug it reads `false` because the server never even requests one.
- [ ] **Step 6.** Add `no_certificate` to arm (b)'s `wantLeaves` (§0.3): under the bug arm (b) reads `no_certificate=1` **alongside** `handshake=1`, a pair that cannot occur on a `require_client_certificate: true` chain.
- [ ] **Step 7. Record cell 8 accurately** (§0.11): **BOTH arms are RED at the tip**, and arm (b) is red for the discriminating reason. Cell 8's job is narrower than `SPEC.md` §12 claims.

**Commit.**

---

### Task 8: ⚠️ NC 7 — WRITE THE AUTHENTICATION BYPASS DELIBERATELY, PROVE IT REDDENS, REVERT UNDER A GUARD

**Files:** none committed. **This is the roster's most important cell and no inherited document asked for it.**

- [ ] **Step 1.** `sha256sum internal/tls/config.go` into scratch.
- [ ] **Step 2.** Write §0.1's bug: pre-build the alternate ONCE at the install anchor (`cfg.Clone()` with `NextProtos = nil` at install time) and return that same pointer from the callback.
- [ ] **Step 3.** Drive Task 7. **Expected, MEASURED at this stage:** arm (b) reddens at BOTH TLS versions with `hsCompleted=true served=true negotiated="" certSent=false`, `no_certificate=1`, `handshake=1`, `connection_error=0` — a client with **no certificate** getting a full application round trip through a `require_client_certificate: true` chain. Arm (a) reddens too, on `certSent=false`.
- [ ] **Step 4.** Record the two configs side by side at handshake time: `PREBUILT alt.ClientAuth=NoClientCert alt.ClientCAs==nil? true` vs `PER-HANDSHAKE clone.ClientAuth=RequireAndVerifyClientCert clone.ClientCAs==nil? false`.
- [ ] **Step 5. ⚠️ COMMIT ANYTHING PENDING FIRST** — `git checkout --` restores from HEAD and will wipe uncommitted work. Then revert and `sha256sum -c` the capture. Prove the tree clean with `--untracked-files=all`, and **check the main repo root too**.

**No commit** (measurement only) — the RESULT is recorded in `PROGRESS.md`.

---

### Task 9: The production comment sites of §7

**Files:** Modify `internal/tls/config.go`, `internal/tls/doc.go`, `internal/listener/manager.go`

- [ ] **Step 1.** `config.go:25` — describes a phase-03 design that no longer exists; rewrite to say the chain's config goes directly to `stdtls.Server` and that THIS row installs the first real `GetConfigForClient`, for ALPN fallback only.
- [ ] **Step 2.** `doc.go:5` — same correction.
- [ ] **Step 3.** `manager.go:138` — ⚠️ **factually correct as history; do NOT revert.** APPEND that a per-chain `GetConfigForClient` returns at phase 95 for ALPN fallback, which is not the phase-03 SNI dispatch.
- [ ] **Step 4.** `config.go:264` (`NewQUICDownstreamConfig` doc) — add one clause: the phase-95 fallback is deliberately NOT installed here, because `negotiateALPN`'s RFC 9001 §8.1 guard is predicated on a non-empty `serverProtos` (§2.2).

**Commit.**

---

### Task 10: Fixture `0120` — both YAMLs, same key, same value, same position

**Files:** Modify `test/fixtures/0120-tls-connection-error/envoy.yaml`, `…/envoy-go.yaml`

- [ ] **Step 1.** Add `alpn_protocols: ["h2", "http/1.1"]` as the first key of `common_tls_context` on BOTH sides — the `0004`/`0079`/`0080`/`0119` shape. Insertion anchors verified at this tip: `envoy.yaml:68` and `envoy-go.yaml:57`, both the line `common_tls_context:`.
- [ ] **Step 2. Boot BOTH sides and confirm arms (i)-(v) are UNMOVED.** ⚠️ Already measured (§0.7): the five-arm delta sequences are **byte-identical** with and without the key, on both sides. Re-confirm rather than assume.
- [ ] **Step 3.** ⚠️ Reference container: `inline_string:` only (no `filename:`), PEM indented ONE LEVEL DEEPER than its key, no PEM in comments.

**Commit.**

---

### Task 11: Fixture `0120` — arm (vi), recorded and NOT drive-fatal

**Files:** Modify `test/fixtures/0120-tls-connection-error/driver/driver.go`

- [ ] **Step 1.** Append arm (vi) — valid client cert + ALPN offer `["bogus/9"]` — **LAST** in `driveSide`, so the five existing arms keep their order and their `record()` indices.
- [ ] **Step 2. ⚠️ THE ARM MUST RECORD ITS OUTCOME, NOT FAIL ITS DRIVE** (§0.10). A failing drive hits the runner's `t.Fatalf("subj drive: %v", err)` and **aborts the subtest before `AssertStats` runs at all** — making the cross-side assertion the arm exists for into dead code, on exactly the side under test. Anchor on the enclosing runner function: **the literal occurs TWICE (`:1277`, `:2078`)**.
- [ ] **Step 3.** ⚠️ `fixture.TB` has no `Logf` — use `log.Printf` to record.

**Commit.**

---

### Task 12: Fixture `0120` — the RE-MEASURED pins and the arm table

**Files:** Modify `test/fixtures/0120-tls-connection-error/driver/driver.go`

- [ ] **Step 1.** `wantConnectionError` STAYS **3**; `wantHandshake` **1 -> 2**. ⚠️ **Not inherited — MEASURED on a booted pair against the digest-pinned reference** (§0.7).
- [ ] **Step 2.** Extend the `⚠️ THESE ARE ARM ARITHMETIC` table with the measured row `(vi) valid + bogus ALPN   +0   +1`, and keep the warning.
- [ ] **Step 3.** ⚠️ **If the §6.3 over-firing control lands as a SEVENTH permanent arm the pins become `3` / `3`, not `3` / `2`** — measured. Decide ONE and make the table match.
- [ ] **Step 4.** Confirm the subject is RED at the tip with exactly two errors: `subject: ssl.connection_error = 4, want 3` and `subject: ssl.handshake = 1, want 2`.

**Commit.**

---

### Task 13: ⚠️ Fixture `0120` — REPLACE `SPEC.md` §12 CELL 13, WHICH CANNOT FAIL

**Files:** Modify `test/fixtures/0120-tls-connection-error/driver/driver.go`. ⚠️ **Split candidate** (§1.3).

- [ ] **Step 1. Record the refutation** (§0.2): removing `alpn_protocols` from the SUBJECT side only leaves the fixture **GREEN** (`RC=0`, `FAILCOUNT=0`, both sides `3`/`2`); removing it from the REFERENCE side only is RED but **byte-identical to the tip-bug red**, i.e. confounded. **No arm can see the YAML edit through `ssl.connection_error`/`ssl.handshake`** — arms (i)-(v) send no ALPN and arm (vi) sends a non-overlapping one, so a server with the key plus the fallback and a server with no key at all behave identically on all six.
- [ ] **Step 2. Land the MEASURED repair:** an arm offering an **OVERLAPPING** list `["h2","http/1.1"]` that pins **`ConnectionState().NegotiatedProtocol` per side**. Measured discrimination: with both YAMLs edited, reference `"h2"` / subject `"h2"`; with the subject YAML reverted, reference `"h2"` / subject **`""`** — while the stat counters stay `3`/`2` on both sides.
- [ ] **Step 3.** ⚠️ **If the overlap arm is NOT landed, record cell 13 as UNFIREABLE in `PROGRESS.md` and delete it from the roster.** Do not ship a guard that cannot fail (`topic_gate_hygiene`).
- [ ] **Step 4.** Re-measure the pins again if this adds an arm (§0.7).

**Commit.**

---

### Task 14: Fixture `0120` — `expectations.yaml` and `README.md`

**Files:** Modify `test/fixtures/0120-tls-connection-error/expectations.yaml`, `…/README.md`

- [ ] **Step 1.** `expectations.yaml` ord table (`:63-72`) gains the new row(s); `:118-119` gains the re-measured totals.
- [ ] **Step 2.** `README.md` arm table (`:77-83`) and pin block (`:182-187`) likewise.
- [ ] **Step 3.** Assert `+0` fixtures (`0120` EXTENDED, `0121` still FREE, total **122**), `+0` BackendKinds (tail STAYS **38**), port unchanged (**10126**), and `runner_test.go` **BYTE-UNTOUCHED**.
- [ ] **Step 4.** Re-run the §3e extractor: **dirs 122 = imports 122, both `comm` directions EMPTY**, split 98 `driver/` + 24 `inputs/`. ⚠️ **NC the extractor on a SCRATCH COPY** — rename one import and prove it fires in BOTH directions. ⚠️ **A count-only check is VACUOUS: the count stays 122 under a rename.**

**Commit.**

---

### Task 15: `ADR-0317` — §Decision + §Consequences, and DISARM the house guard

**Files:** Modify `docs/envoy-go/DECISIONS.md`

- [ ] **Step 1.** Append §Decision + §Consequences **IN PLACE, AFTER** the retained italic footer `*§Decision and §Consequences follow at the phase-95 IMPL.*`. DECISIONS.md is **append-only**: ADR-0316 §Consequences (vi) at `:18862` stays VERBATIM.
- [ ] **Step 2. DISARM** the house guard by flipping the token in place at `DECISIONS.md:18876`. ⚠️ **VERIFY BY LINE AND BY ADR** — resolve the hit backward to `## ADR-0317` with `awk 'NR<=<HIT> && /^## ADR-/ {h=$0; n=NR} END {print n, h}'`. ⚠️ **The ADR-0231 decoy at `:14866` is a DIFFERENT MATCHER and stays byte-untouched.** ⚠️ **NEVER gate on the unanchored form nor the middle-ground `^\*\*Status:\*\*.*PROPOSED` form** (which reads **23**), and **DO NOT WRITE A COUNT OF EITHER MATCHER IN PROSE THE GREP MATCHES** — the phase-93 SPEC falsified itself doing exactly that.
- [ ] **Step 3.** Record in §Consequences: the bypass CONFIRMED BY EXECUTION (§0.1), the TCP-only rationale, the `+0` stat surface, and that no SDS+ALPN fixture exists (`SPEC.md` §8) — **absent coverage, stated not implied**.
- [ ] **Step 4.** `^---$` STAYS **216** if no separator is added; `^## ADR-` STAYS **316**; tail STAYS `ADR-0317`; next-free STAYS `ADR-0318`.

**Commit.**

---

### Task 16: `BEHAVIOR_CONTRACT.md:1944` — WITHIN-LINE, BY LITERAL TEXT

**Files:** Modify `docs/envoy-go/BEHAVIOR_CONTRACT.md`

- [ ] **Step 1.** ⚠️ **Anchor on the literal clause, NEVER on the line number.** The clause `is not surfaced to the fixture driver in phase 03` occurs **exactly once** (verified at this tip). The line is **357** characters — ⚠️ NOT the "one enormous paragraph line" earlier routers called it; that is `:1971` at **5098** (`SPEC.md` §0.4).
- [ ] **Step 2.** The replacement states: (a) phase 95 is the phase that promise named; (b) the MEASURED reference rule for all three offer classes; (c) that the fixture opt-in is `0120` arm (vi), asserted cross-side on `ssl.handshake`/`ssl.connection_error`; (d) that the fallback is TCP-only and QUIC's RFC 9001 §8.1 rejection is **PARITY, not a departure**. **No new departure is named — after the row there is none.**
- [ ] **Step 3. ⚠️ DO NOT TOUCH `:1961`** (736 chars, the ALPN-consumer paragraph). It needs no edit: post-fix a mismatched connection presents `NegotiatedProtocol == ""` at the HCM boundary — byte-identical to the already-passing no-ALPN arm — and `hcm/filter.go:104-106` dispatches `""` to the H1 driver, which is what the reference does.
- [ ] **Step 4. KEEP THIS EDIT ATOMIC WITH THE CODE** (`SPEC.md` §15 item 7). A contract describing behaviour the tree does not yet have is worse than a stale one.
- [ ] **Step 5.** Stat surface **+0** ⇒ by the ledger's own convention at `:5130`, **NO chain entry**. Quote the surface as a DELTA, never as an absolute — three inconsistent absolutes are live at one tip.

**Commit.**

---

### Task 17: `ROADMAP.md` row 95 -> `done`, `STATE.md`, `PROGRESS.md`

**Files:** Modify `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/STATE.md`; create `PROGRESS.md`

- [ ] **Step 1.** Flip row 95 `in-progress` -> `done`. ⚠️ **Sentinel check (1) then goes SILENT** — and with checks (2) and (3), **the sentinel STILL DOES NOT FIRE, because check (2) reads SIX.** Do NOT "fix" those six; do NOT delete a candidate line; the history is `0 -> 1 -> 3 -> 4 -> 5 -> 6` across ~40 phases.
- [ ] **Step 2. ⚠️ THE ROW SUMMARY MUST CARRY NO UNESCAPED `|`.** Count fields (want **8**) under BOTH the naive and the escape-aware form, BEFORE and AFTER installing. **Reword a pipe away rather than escaping it.** Baseline: escape-aware malformed rows are IDs **57** and **69** only; rows 94 and 95 read 8/8 under both forms.
- [ ] **Step 3.** ⚠️ **NEVER RE-SPELL A SENTINEL MATCH PHRASE INSIDE A SENTINEL WINDOW** — the phase-94 rule: the sentence asserting the sentinel could not move MOVED IT, six -> seven.
- [ ] **Step 4.** `STATE.md` rolled IN PLACE; `PROGRESS.md` created (the lifecycle's IMPL artefact; **no `REVIEW.md`** — standing departure, NAMED not claimed).
- [ ] **Step 5.** Re-run all three checks, all four NCs and the check-(2) positive control, and record ACTUAL output. ⚠️ **NC SHAPES CHANGE ACROSS A ROW FLIP — NEVER INHERIT ONE.** After the flip NC-A reads **ONE** line, not two.

**Commit.**

---

### Task 18: Full verification sweep — THE SIX-GATE POSTURE

**Files:** none. **Name departures; do not claim compliance.**

- [ ] **(a) Differential — 122/122**, `-count=1` NOT optional (~400s). **ASSERT THE FIXTURE SET BY NAME, BOTH DIRECTIONS**, against the fixtures on disk. ⚠️ `-race` on this suite is **VACUOUS** — the subject is an unraced subprocess.
- [ ] **(b) Non-Docker sweep — 235 packages**, gated on `PIPESTATUS[0]` **plus a SET RECONCILIATION**. ⚠️ `go test ./...` drives Docker: exclude BOTH drivers with `go list ./... | grep -vE '/test/differential$|/test/conformance/h2spec$'`.
- [ ] **(c) h2spec** — `95 tests, 94 passed, 1 skipped, 0 failed`.
- [ ] **(d) Fuzzers** — **56 targets / 48 files**, unchanged (the row consumes no new config field).
- [ ] **(e) The ANCHORED panic gate** `^panic:|DATA RACE|SIGSEGV` = **0**, **and PROVEN LIVE** over an input known to trip it (a gate that reads 0 has not been shown to work).
- [ ] **(f) `REVIEW.md`** — absent, standing departure.
- [ ] **(g) ⚠️ ASSERT THE GUARDED NON-SITES BYTE-UNTOUCHED** — the nine history-class sites of §0.5 and `quic_negative_test.go:92` (§0.17).
- [ ] **(h)** `gofmt -l` (gate on OUTPUT) and `golangci-lint` per touched package, US-locale misspell swept first.

**Commit.**

---

## 6. The NC roster, CORRECTED

`SPEC.md` §12's thirteen cells, with the four this stage changed marked. ⚠️ **NEUTRALISE, NEVER REVERT** — every NC must leave the package compiling, and the NC itself must be checked EXECUTABLE.

| # | pin | neutralisation | must read | status at this stage |
|---|---|---|---|---|
| 1 | §5.1 flipped test | the tip itself | RED at the mismatch arm | **MEASURED** — exactly one test transitions (§0.15) |
| 2 | §5.1 control arm | callback fires unconditionally | RED (`negotiated ALPN = "", want "http/1.1"`) | **MEASURED RED** |
| 3 | §5.2 row 7 | the tip itself | RED | **MEASURED RED** |
| 4 | §5.2 row 8 | the tip itself | **GREEN** — not a red-flip pin | **MEASURED GREEN** |
| 5 | §5.2 row 8, isolating | implement the `http/1.1`-vs-`h2` shape ONLY | row 8 GREEN, row 7 RED | ⚠️ **SCOPE CORRECTED** (§0.8): valid PRE-fix only — post-fix the callback intercepts row 8 first, so only row 7 discriminates |
| 6 | §5.2 `wantLeaves` | set one row's map to the wrong leaf | RED | ⚠️ **FIGURE CORRECTED** (§0.4): the map is `{handshake:1, no_certificate:1}` |
| 7 | §5.3 arm (b) | **pre-build at the anchor — §0.1's bug, written deliberately** | RED | ⚠️ **EXECUTED THIS STAGE** (§0.1) — the bypass reproduces and the client is **SERVED** |
| 8 | §5.3 arm (a) | do not implement the row | RED | ⚠️ **UNDER-STATED** (§0.11): BOTH arms are red at the tip |
| 9 | §5.4 `len(NextProtos)==0` guard | remove the guard | RED | **MEASURED RED** |
| 10 | §5.4 base-config assertion | empty `cfg.NextProtos` in place | RED | ⚠️ **INVISIBLE without a post-drive base re-read** (§0.9); also a live DATA RACE |
| 11 | §6.2 arm (vi) | the tip itself, subject side | RED (`+1/+0` vs a `+0/+1` pin) | **MEASURED RED**, exactly two errors |
| 12 | §6.3 over-firing control | drive arm (vi) twice | `handshake +2` | **MEASURED** — and it moves the pins to `3`/`3` |
| 13 | the `0120` YAML edit | remove `alpn_protocols` from ONE side | RED cross-side | 🔴 **REFUTED — VACUOUS** (§0.2). Replaced by the measured overlap arm, or struck. |

⚠️ **Two cells written as specified would have FAILED AGAINST A CORRECT IMPLEMENTATION** (6 and, via §5.3's error-string instruction, 7/8), and **one could never fail at all** (13). That is the single most important result of this stage.

---

## 7. Counts — RE-DERIVED AT THIS PLAN'S OWN TIP (`9ed6a620`)

Discharges `SPEC.md` §15 item 9. Every figure RUN, under a NAMED matcher. ⚠️ `feedback_brief_citations_not_evidence` applies to every number in `SPEC.md`, in `next-prompt.txt`, and in this table.

| axis | at this tip | after this PLAN | after the ROW |
|---|---|---|---|
| `ROADMAP.md` lines / data rows (`want`) | **245 / 127** | **unchanged** | unchanged (row 95 FLIPS, no ADD) |
| sentinel (1) / (2) / (3) | one line / **SIX** / SILENT | **unchanged** | SILENT / SIX / SILENT — **still no fire** |
| `DECISIONS.md` lines | **18902** | **BYTE-UNTOUCHED** | grows (§Decision + §Consequences) |
| `^---$` / `^## ADR-` / bare `^## ` | **216 / 316 / 324** | unchanged | 216 / 316 / 324 |
| tail ADR / next-free | **ADR-0317 / ADR-0318** (`^## ADR-0318` reads **0**) | unchanged | unchanged |
| house guard `^> \*\*STATUS: PROPOSED` | **ARMED**, `:18876`, resolved BACKWARD to `## ADR-0317` at `:18874` | **STAYS ARMED** | **DISARMED** |
| ADR-0231 decoy `^\*\*Status:\*\* PROPOSED` | **1** at `:14866` | untouched | untouched |
| `BEHAVIOR_CONTRACT.md` lines / `:1944` / `:1961` / `:1971` chars | **5989 / 357 / 736 / 5098** | **BYTE-UNTOUCHED** | +0 lines (WITHIN-LINE at `:1944`) |
| stat surface | *(DELTA only — three inconsistent absolutes are live)* | **+0** | **+0**, and **NO ledger row** (`:5130`) |
| `STATE.md` / `STATE_HISTORY.md` lines | **65 / 550** | rolled in place / **550 -> 552 (+2)** | — |
| archive guard strict / parenthetical / loose | **163 / 62 / 225** (`163 + 62 = 225` exactly) | **163 (DELTA 0) / 63 / 226** | — |
| phase dirs | **136** | unchanged | unchanged |
| fixtures | **122**, tail `0120`, `0121` FREE | unchanged | **122** — `0120` EXTENDED |
| fixture extractor | dirs **122** = imports **122**, both `comm` directions EMPTY, **98 `driver/` + 24 `inputs/`** | unchanged | unchanged |
| BackendKind tail | **38** (`fixture.go:614`) | unchanged | **38** |
| fuzzers | **56 targets / 48 files** | unchanged | unchanged |
| `go.mod` require entries | **67** (⚠️ the `grep -cE '^\s+[a-z0-9./-]+ v[0-9]'` form reads **62**) | unchanged | unchanged |
| `go list ./...` | **237** (**235** excluding the two Docker drivers) | unchanged | unchanged |
| `-family row` | **96 occurrences / 68 lines** (⚠️ `--` before the pattern) | unchanged | unchanged |
| malformed rows naive / escape-aware | **17 / 2** (IDs **57**, **69**; file lines 119, 131) | unchanged | unchanged |
| `internal/tls/config.go` lines | **582** | unchanged | **629** (MEASURED, +47) |
| `./internal/tls/` + `./internal/listener/` | `RC=0`, **RUN=326** (178 + 148), FAIL 0 | unchanged | RUN grows; FAIL 0 |

⚠️ **NO ABSOLUTE STAT-SURFACE FIGURE IS QUOTED ANYWHERE IN THIS DOCUMENT** (ADR-0316 §Consequences (x)).
⚠️ **The malformed-row figure reconciles ONLY under the escape-aware form, and that command is itself breakable:** `sed 's/\\|//g' F | awk -F'|' '/^\| *[0-9]/ && NF!=8'` with **NO file argument to awk** — passing the file makes awk IGNORE STDIN and print the NAIVE 17 under the escape-aware label. **CONFIRMED broken-when-misused at this tip.**
⚠️ **`want=127` does NOT live in `ROADMAP.md`** — the only `want=` literals there are prose. The sentinel's `want` lives in `next-prompt.txt`.

---

## 8. Cost — MEASURED and ESTIMATED, labelled separately

**MEASURED** (from prototypes applied and reverted this stage): `internal/tls/config.go` **+47 / -0**. The §4 code compiles, needs no new import, and passes.

**ESTIMATED** (§1.3): **≈ +989 / -127** total, **≈ +812** counting only `.go`. Both thresholds clear.

⚠️ **`git diff --stat` is a SUM, not additions** — every cost figure here comes from `--numstat`.

---

## 9. Deferred — newly surfaced by THIS PLAN, none chartered

- **The nine history-class `GetConfigForClient` comment sites** (§0.5) are GUARDED, not rewritten. A future row may consolidate them; this one must not, because rewriting history-as-history is how the phase-03 record gets lost.
- **`SPEC.md` §12 cell 13's replacement** (§0.2) lands as a fixture arm, but the deeper gap remains: **no fixture asserts `NegotiatedProtocol` cross-side at all**, so a whole class of ALPN divergence is unguarded. Not chartered.
- **No fixture combines SDS with `alpn_protocols`** (`SPEC.md` §8) — `0103` and `0108`-`0111` carry SDS and no ALPN; the five ALPN fixtures carry no SDS. **Recorded as ABSENT COVERAGE at ADR-0317 §Context ¶8, deliberately not chartered.**
- **The driver-owned receiver port race** (~36 driver files) stays **BANKED** — still the most defensible next pick if a full differential run aborts again.
- ⚠️ **`next-prompt.txt`'s recursive-grep blindness note is under-scoped** (§0.14) — it is true in the controller shell and false in a subagent shell. A future roll should say SHELL as well as TOOL.

---

## 10. Cite hygiene — what the IMPL must NOT inherit

- ⚠️ **`internal/filter/hcm/chain.go` DOES NOT EXIST** — it is `internal/filter/http/chain.go`. But `actions.go` IS in `hcm`.
- ⚠️ **`config_test.go:1193-1204`** is cited by `SPEC.md` §0.8; `:1193` is the last line of a DOC COMMENT and the assertion is **`:1203-1204`** (§0.15).
- ⚠️ **`SPEC.md` §5.1 calls the control arm `:149-159`;** its `Errorf` is at **`:158`**.
- ⚠️ **`runner_test.go:1281`** does not carry the subject-drive fatal; **`:1277` and `:2078` do** (§0.10).
- ⚠️ **The `129 196 221 230 249 259` return set is WRONG** — it is `128 195 220 229 248 258`, and a uniform `+1` skew is a SIGNATURE worth suspecting whenever a cited set is off by one.
- ⚠️ **`0004` is NOT the only fixture with downstream `alpn_protocols` — FIVE carry it.**
- ⚠️ **`ENVOY_TARGET.md` is at `docs/envoy-go/ENVOY_TARGET.md`,** not the repo root. The pin is `envoyproxy/envoy:contrib-v1.37.2`, digest `7edd5b0fd763…` — **verified in use this stage.**
- ⚠️ **`BOOTSTRAP_PROMPT.md` §5 appears VERBATIM TWICE** (§5 and §11); the second is a duplicate.

---

## 11. `SPEC.md` §15 coverage — every owed item

| # | owed | discharged |
|---|---|---|
| 1 | a task count DERIVED, split gate checked | §1.3 — **18 tasks**, ≈ +812 `.go` lines; NOT split, reasoning stated |
| 2 | **BOOT BOTH SIDES, re-measure all six `0120` arms per side** | §0.7 — booted against the digest-pinned reference; full per-side per-arm table; `3`/`2` CONFIRMED; arms (i)-(v) byte-identical with and without the key |
| 3 | the PACKAGE SWEEP, not a selector | §0.15 — `RC=0`, **RUN=326**, FAIL 0; the three `internal/tls` `NextProtos` sites MEASURED, not assumed |
| 4 | **row 7 RED and row 8 GREEN at the tip, BEFORE the fix** | §0.4 — both measured at the tip; **and the SPEC's "rest 0" figure REFUTED** |
| 5 | **LAND NC 7** | §0.1 + Task 8 — the bug was written, driven, and the bypass reproduced at both TLS versions; reverted under `sha256sum -c` |
| 6 | prove the mismatch branch is REACHED with `panic()` | §0.8 — fires on 2 of 4 drives; stack confirms the `handshake_server.go:169` dispatch anchor |
| 7 | keep the contract edit ATOMIC with the code | Task 16 — and §1.2 measures that a PLAN may not make it |
| 8 | fixture set BY NAME both directions + the §3e extractor NC | Task 14 + §7 — **122 = 122**, both `comm` directions empty, extractor NC fired BOTH ways on a scratch copy; ⚠️ a count-only check is VACUOUS |
| 9 | re-derive every §10 count at this tip | §7 — every axis re-run; **one REFUTED** (`STATE_HISTORY.md` +2, §0.12) |

### ⚠️ The ONE deliberate deviation, stated rather than made silently

**Item 5 says "LAND NC 7"; this PLAN RUNS NC 7 and lands NOTHING.** `next-prompt.txt`'s closing owed-list says *"schedule NC 7"* — the two instructions disagree. **The measured precedent settles it:** across FOUR precedent PLAN commits (§1.2) no PLAN touches any file under `internal/` or `test/`, so NC 7 CANNOT land as committed code at this stage without breaking the measured scope. It was therefore **executed as a throwaway probe and reverted under a `sha256sum -c` guard** — which is exactly what item 5's own second clause (*"then revert it under a `sha256sum -c` guard"*) describes. **The result is recorded (§0.1); the code is not.** Task 8 schedules the re-run at the IMPL.

---

## 12. Self-review — run against the SPEC with fresh eyes

- **Does every task end in a commit?** Yes, except Task 8, which is measurement-only and says so.
- **Is any pin satisfiable by doing nothing?** Row 8 alone would be (§0.8) — stated, and paired with row 7. Cell 13 was (§0.2) — refuted and replaced.
- **Does any pin fail against a CORRECT implementation?** Two would have (§0.3 error string at TLS1.2, §0.4 `wantLeaves`) — both corrected here.
- **Is any anchor non-unique?** Two are (§0.6 the returns, §0.10 the runner fatal) — both re-anchored on enclosing symbols in §4.1.
- **Is anything claimed that was not run?** The post-fix subject reading in §0.7 is a PROXY (the subject without `alpn_protocols`), and is labelled as one. The full 122-fixture suite, h2spec and the 235-package sweep are **scheduled at Task 18, not run** — a PLAN runs no gates.
