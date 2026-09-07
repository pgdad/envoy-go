# Phase 95 — `tls-alpn-mismatch-fallback` — SPEC

Lifecycle state **1 -> 2**. Predecessor: `BRAINSTORM.md` (314 lines) at `7f568db2`, plus the router correction at `abc309d7`. This stage discharges the nine items of `BRAINSTORM.md` §10 and refutes its predecessor **eleven** times by execution.

**Charter, unchanged from `ROADMAP.md:157`:** on the TCP downstream path, a client that offers a NON-EMPTY ALPN list overlapping none of the chain's `alpn_protocols` must COMPLETE the handshake with no protocol selected — matching the pinned reference — instead of aborting with `no_application_protocol`.

---

## 0. What this stage refuted

Eleven, each by execution at this tip (`abc309d7`), not by reading. Items 1, 2 and 7 are load-bearing: one changes the DESIGN, one changes the SCOPE of this very commit, one changes the TEST SET.

**0.1 ⚠️ THE BRAINSTORM'S "IT CAN PRE-BUILD" IS REFUTED BY MUTATION ORDER, AND THE FAILURE MODE IS AN AUTHENTICATION BYPASS.** §3.3 says the SPEC may pre-build the alternate config once per chain because *"`NextProtos` is immutable after build."* That is true of `NextProtos` and false of the config. The mandated install point is the ENTRY (`config.go:57`, after the `:53` call and its `:54-56` error check). `cfg.ClientCAs` and `cfg.ClientAuth` are assigned **later**, at `:94-95`, inside the `installPool` closure invoked from the validation-context switch at `:99`. A clone taken at `:57` therefore carries `ClientCAs == nil` and `ClientAuth == NoClientCert` (the zero value). Handing that clone to a mismatched-ALPN client on a chain configured `require_client_certificate: true` would admit it **with no client-certificate verification at all**. The file's own comment at `:85-89` warns about exactly this class of mistake for the nil-pool case. Pre-building at the six EXITS instead would reintroduce the §0.4 six-exit hazard the entry placement exists to remove. ⇒ **Per-handshake `Clone()` is not a convenience; at the mandated placement it is the only correct option.** See §3.

**0.2 ⚠️ THE SPEC STAGE MAY NOT TOUCH `BEHAVIOR_CONTRACT.md`, AND THE ROUTER'S OWED-LIST ITEM 5 SAYS IT MUST.** `next-prompt.txt` §10 item 5 and its closing owed-list both instruct this stage to *"Extend `BEHAVIOR_CONTRACT.md:1944` WITHIN-LINE."* Measured across **five consecutive SPEC commits** — `975e527e` (93), `13ad0aa0` (92), `8f7bdaf0` (91), `682b4734` (90), `307f2e3d` (94) — **every one leaves `BEHAVIOR_CONTRACT.md` and `ROADMAP.md` BYTE-UNTOUCHED**; the uniform scope is `DECISIONS.md` + the phase dir's `SPEC.md` + the three roll files. The contract edit is the IMPL's to land, PINNED here (§11), which is also what the phase-94 SPEC did (its §11 pinned contract edits it did not make). `reference_stage_file_scope_is_measurable` and `reference_plan_schedules_edits_to_a_byte_gated_file` both fire. **Method note 21 is still satisfied**: the governing document this stage's measurement reaches is `ADR-0317 §Context`, drafted here.

**0.3 ⚠️ FOUR OF THE CITED PRODUCTION ANCHORS ARE OFF BY EXACTLY +1, ALL IN ONE DIRECTION.** The six `NewDownstreamConfig` return sites are at **128, 195, 220, 229, 248, 258** — not the `129 196 221 230 249 259` carried by `BRAINSTORM.md` §0.4 and `next-prompt.txt`. At each cited line the content is the closing brace that follows the return. The QUIC builder's return is at **290**, not `:291`. The `NextProtos` append is at **579**, not `:580` — `:580` is a blank line. The `_Aborts` function spans **121-171**, not `121-170`. Verified by direct read, not by an agent's word.

**0.4 ⚠️ `BEHAVIOR_CONTRACT.md:1944` IS NOT "ONE ENORMOUS PARAGRAPH LINE."** The router's read-first item 6 calls it that and warns that `sed`-style line replacement would destroy neighbouring propositions. Measured with `awk 'NR==1944{print length($0)}'`: **357** characters — a single short sentence pair. The enormous line (**5098** characters) is `:1971`, the phase-94 departure paragraph, and the characterisation is inherited from the phase-94 SPEC §11, which said it about `:1971`. The WITHIN-LINE discipline still applies — `:1944` IS one paragraph line and it DOES carry the promise this row redeems — but the stated hazard is a conflation. **Edit by literal text regardless.**

**0.5 ⚠️ `0004` IS NOT THE ONLY FIXTURE CARRYING DOWNSTREAM `alpn_protocols` — FIVE DO.** `BRAINSTORM.md` §7.1 and `next-prompt.txt` method note 13 both say *"exactly ONE."* Measured: `0004-h2-routing` (`envoy.yaml:42` / `envoy-go.yaml:40`), `0079-h2-multiplex-pool/driver/driver.go:295`, `0080-h2-goaway-rotation/driver/driver.go:287`, `0119-grpc-unary-trailers/driver/driver.go:177` — all four TCP, all four `["h2", "http/1.1"]` — plus `0104-http3-downstream-get/driver/driver.go:237` and `:307`, both `["h3"]` on a QUIC listener. **The conclusion survives and is now provable rather than asserted** (§6.4): the tip ENFORCES overlap, so any fixture green at the tip is by construction one whose clients overlap or send nothing — precisely the two cases the fallback no-ops on. `0104` is QUIC and the install is TCP-only.

**0.6 ⚠️ THE `mkDownstreamTSInlineALPN` HELPER'S OWN DOC COMMENT ALSO ASSERTS THE FALSE PARITY, AND THE §3.4 ROSTER MISSES IT.** `tls_handshake_negative_test.go:85-87` says the chain's config *"carries NextProtos and the server ENFORCES ALPN overlap."* The roster enumerates the `:113-120` test doc comment and not this one, in the same file the roster names, in the helper §10 item 3 tells this row to REUSE. `reference_measured_prototype_is_a_lower_bound` fires for a twelfth consecutive row — this time by under-enumerating a COMMENT SITE.

**0.7 ⚠️ ARM (vi) AS SPECIFIED CANNOT DETECT §0.1's BYPASS.** `BRAINSTORM.md` §6.3 specifies arm (vi) as *valid client cert + ALPN offer `["bogus/9"]`*. A valid cert is accepted under both the correct implementation and the §0.1 bug, so the arm passes either way. The discriminating shape is a **WITHHELD** client cert against the same mismatched offer: it must still be REJECTED after the row. §5.3 adds it as a unit arm — cheaper and more isolating than a second fixture arm — and §6.2 keeps arm (vi) as specified for the cross-side property it does test.

**0.8 ⚠️ THE PIN SURFACE IS WIDER THAN THE SELECTOR RUN, AND `internal/tls` IS THE PART IT MISSES.** `BRAINSTORM.md` §6.2 measured 11 tests under a `./internal/listener/` selector. The row's production edit is in `internal/tls`, whose own suite carries `NextProtos` assertions at `config_test.go:176-181`, `:548-553` and `:1193-1204`, and none of them was run under the prototype. They are expected green (the row never changes the BASE config's `NextProtos`) — **expected, not measured.** The PLAN runs both packages.

**0.9 ⚠️ THE TIP'S SELECTOR RUN IS GREEN, WHICH IS THE POINT.** Re-run here at `abc309d7` with no patch: `RC=0`, `RUN=11`, anchored-FAIL **0**, `no tests to run` **0**. `TestNewManager_LiveHandshake_ALPNNegotiationFailure_Aborts` **PASSES at the tip while pinning the wrong side.** That is the row's built-in negative control, now measured on the unpatched tree rather than inferred from the patched one.

**0.10 ⚠️ THE ROUTER'S EVICTION CHECK IS UNDER-SPECIFIED, AND ITS BARE FORMS DO NOT READ ZERO.** `next-prompt.txt` says *"at this close BOTH the strict and the naive form read 0 on `STATE.md`."* Measured at this tip: the bare strict form `^- \*\*prior active-phase:\*\* ` reads **5** and the naive `prior active-phase` reads **6** (the sixth is inside the `last-updated` bullet at `:20`). Only the LABEL-BOUND form reads 0, and only AFTER an eviction. The check that actually discriminates is a PAIR, and at this tip it reads: the evictee's label on `STATE.md` **1**, on `STATE_HISTORY.md` **0**. Those two must SWAP at this close. §13.3.

**0.11 ⚠️ THE DATE TIE HAS MOVED TO THE OLDEST POSITION, SO A DIRECT DATE READ NO LONGER IDENTIFIES THE EVICTEE.** The router records the phase-95 BRAINSTORM shape as *"the oldest is UNIQUE and is ALSO the tail entry"* and asks whether the tie lands at the oldest position — *"READ, DO NOT PREDICT."* Read at `7964f620`: `09-03, 09-02, 09-01, 09-01, 08-31` — tie in the middle, unique oldest at the tail. Read at `abc309d7`: `09-05, 09-03, 09-02, 09-01, 09-01` — **the tie IS now at the oldest position.** A date sort is therefore AMBIGUOUS at this tip and the evictee must be taken by LIST POSITION (the tail), which is the phase-93 IMPL entry. §13.3.

---

## 1. Scope

**This commit touches exactly five files** (the measured SPEC-stage scope of §0.2):

| file | change |
|---|---|
| `docs/envoy-go/DECISIONS.md` | `ADR-0317` §Context only, house `PROPOSED` form, appended at the tail |
| `docs/envoy-go/phases/95-tls-alpn-mismatch-fallback/SPEC.md` | this file, new |
| `docs/envoy-go/STATE.md` | §Current pointer rolled IN PLACE; oldest §Recent entry evicted |
| `docs/envoy-go/STATE_HISTORY.md` | one inline parenthetical-form archive line |
| `next-prompt.txt` | rolled to point at the PLAN |

**BYTE-UNTOUCHED at this stage, asserted not assumed:** `ROADMAP.md` (row 95 STAYS `in-progress`; sentinel `want` STAYS 127), `BEHAVIOR_CONTRACT.md`, every file under `internal/`, every file under `test/`, `go.mod`, `go.sum`. Everything §2-§8 designs is PINNED for the IMPL in §11 and is NOT landed here.

---

## 2. The rule, and the stdlib mechanism — re-verified at go1.26.7

### 2.1 The reference rule (from `BRAINSTORM.md` §2, measured on `envoyproxy/envoy:contrib-v1.37.2`, digest `7edd5b0fd763…`)

On a TCP downstream listener with `alpn_protocols` configured, for a client ALPN offer:

| offer | reference | envoy-go at this tip |
|---|---|---|
| absent | handshake OK, negotiated `""`, connection SERVED | same — **parity** |
| non-empty, ≥1 overlap | handshake OK, negotiated = the overlap | same — **parity** |
| non-empty, NO overlap | **handshake OK, negotiated `""`, connection SERVED**, `ssl.handshake +1`, `ssl.connection_error +0` | **ABORT** with `no_application_protocol`, `ssl.handshake +0`, `ssl.connection_error +1` — **DIVERGENCE** |

BoringSSL's `alpn_select_cb` returns `SSL_TLSEXT_ERR_NOACK` for the third row: no extension is echoed and the handshake proceeds. The HCM's AUTO codec then dispatches on the bytes.

### 2.2 What `crypto/tls` does — read at go1.26.7, `handshake_server.go:334-361`

```
334  func negotiateALPN(serverProtos, clientProtos []string, quic bool) (string, error) {
335      if len(serverProtos) == 0 || len(clientProtos) == 0 {
336          if quic && len(serverProtos) != 0 {
337              // RFC 9001, Section 8.1
338              return "", fmt.Errorf("tls: client did not request an application protocol")
339          }
340          return "", nil
341      }
342      var http11fallback bool
...      // exact-equality overlap scan; returns the match
353      // As a special case, let http/1.1 clients connect to h2 servers ...
357      if http11fallback {
358          return "", nil
359      }
360      return "", fmt.Errorf("tls: client requested unsupported application protocols (%q)", clientProtos)
361  }
```

Three facts this design rests on, each read rather than assumed:

- **`:335` is the lever.** With `serverProtos` empty the function returns `("", nil)` unconditionally on the TCP path. Emptying `NextProtos` for one handshake reproduces the reference's NOACK exactly, without touching the negotiation code.
- **`:336` is why the install must be TCP-only.** The QUIC guard is itself predicated on `len(serverProtos) != 0`. Applying the same fallback on the QUIC path would make that guard VACUOUS and silently disable RFC 9001 §8.1 enforcement — the fallback would not merely be unnecessary there, it would REMOVE a mandated rejection. The BRAINSTORM justified TCP-only by citing the RFC and the landed test; this is the mechanism.
- **`:342-358` is `http11fallback`.** A client offering only `http/1.1` to an `h2`-only server is already let through with no protocol (Go issue 46310). So the subject ALREADY matches the reference on that ONE shape and diverges on every other non-overlapping offer.

### 2.3 The callback contract — `handshake_server.go:167-175`, `common.go:1009`

```
167  if c.config.GetConfigForClient != nil {
168      chi := clientHelloInfo(ctx, c, clientHello)
169      if configForClient, err = c.config.GetConfigForClient(chi); err != nil {
170          c.sendAlert(alertInternalError)
171          return nil, nil, err
172      } else if configForClient != nil {
173          c.config = configForClient
174      }
175  }
```

- Dispatched **exactly once**, on `c.config` as it stood BEFORE any replacement. Returning `(nil, nil)` leaves `c.config` untouched — the no-op path is a genuine no-op with no second entry.
- `Config.Clone()` **does** copy the `GetConfigForClient` field (`common.go:1009`). Because of the single dispatch above this cannot recurse; the design nils it on the clone anyway so the invariant is visible at the call site rather than inferred from stdlib internals.
- Returning a non-nil error would send `alertInternalError`. **The callback in §4 never returns a non-nil error**, which is why it has no error path to test.
- `tls.go:103-104`'s "neither Certificates, GetCertificate, nor GetConfigForClient set" check lives in `tls.Listen`/`NewListener`. Measured: `stdtls.Listen`/`NewListener` appear **only in tests** under `internal/`, never on the production downstream path, which uses `stdtls.Server` at `manager.go:1337`. Installing the callback therefore cannot mask a missing-certificate misconfiguration.

---

## 3. Placement, and the pre-build question DECIDED

### 3.1 The install point

**`internal/tls/config.go`, immediately after `:56`** — that is, after

```
53      cfg, err := commonTLSContextToConfig(ctx.GetCommonTlsContext(), baseDir, "downstream", provider)
54      if err != nil {
55          return nil, err
56      }
```

Two properties make this the correct anchor, both measured:

1. **All six exits return the SAME pointer.** `awk` over `42-259` finds exactly six `return &DownstreamConfig{...}` statements (128, 195, 220, 229, 248, 258) and every one is byte-identical: `return &DownstreamConfig{TLSConfig: cfg}, nil`. `cfg` is BOUND ONCE, at `:53`, and never rebound anywhere in the function — only its fields are mutated (`:94`, `:95`). One mutation at `:57` therefore reaches all six exits with certainty. (The check was NC'd: the same matcher fires on `:568`, `:575`, `:579`.)
2. **The QUIC builder does not pass through it.** `NewQUICDownstreamConfig` (`:268`) makes its OWN `commonTLSContextToConfig` call at `:286` and returns its own `cfg` at `:290`. It is untouched, which §2.2 shows is mandatory, not merely tidy.

⚠️ **"At the return" remains not a placement** (`next-prompt.txt` §3f, `BRAINSTORM.md` §0.4): a patch inserted before the FIRST return landed inside the SDS validate-mode arm, built, booted, and changed nothing.

### 3.2 Per-handshake `Clone()` — the decision, and why the alternative is unsound

**DECIDED: clone per mismatched handshake. Do NOT pre-build a per-chain alternate.**

The reason is §0.1 and it is not a performance argument. At the mandated `:57` anchor, `cfg.ClientCAs` and `cfg.ClientAuth` have **not yet been assigned** — `installPool` writes them at `:94-95`, reached from the validation-context switch at `:99`. A snapshot taken at `:57` would carry `ClientCAs == nil` and `ClientAuth == NoClientCert`. Serving that config to a mismatched-ALPN client on a `require_client_certificate: true` chain is an **authentication bypass on the mismatch path**: mandatory mTLS silently becomes anonymous access for any client that offers a protocol the listener does not advertise.

The closure reads `cfg` at HANDSHAKE time, so it necessarily observes the completed config — including `:94-95`, including any field a future phase adds after `:57`, and including any future in-place SDS rotation. **A per-handshake clone is correct by construction; a build-time snapshot is correct only as long as nobody adds a field assignment after the anchor** — and this row's own anchor already sits before two.

Cost of the decision: one shallow struct copy on the mismatch path only. `crypto/tls` documents `Clone` as safe on a `Config` in concurrent use. Against a full TLS handshake the copy is immaterial, and the mismatch path is by construction the rare one.

Recording the alternatives rejected: pre-building at each of the six exits reintroduces the six-exit hazard; deferring the install to `manager.go` after the chain is fully built would put TLS negotiation policy in the listener package and would have to be repeated at all three build sites (`:617`, `:631`, `:713`), one of which is the QUIC site that must NOT get it.

### 3.3 Why the predicate deliberately subsumes `http11fallback`

The callback fires whenever the offer is non-empty and overlaps nothing — which INCLUDES the `http/1.1`-vs-`h2`-only shape the stdlib already lets through. The observable outcome is identical either way (`negotiated == ""`, handshake completes), so the superset is harmless, and replicating the stdlib special case in our predicate would add a branch with no distinguishable behaviour and no way to test it. **This is a deliberate choice, and §5.2's control row is what pins it** — the control must stay green both before and after, proving the table cannot be satisfied by the stdlib alone.

### 3.4 The overlap helper, and the sibling that is not reused

`internal/listener/listenerfilter/chainmatch.go:293` already has `alpnMatchAny(want, offered []string) bool` with the identical body. It is **unexported**, so reuse would require exporting it; and `internal/tls` importing `internal/listener/listenerfilter` — which `go list -deps` confirms has no envoy-go internal dependencies, so no cycle would result — would still widen a package's public surface to share a six-line loop, and the two helpers answer different questions (chain SELECTION vs negotiation FALLBACK). **A local unexported helper in `internal/tls` is specified.** `reference_grep_for_sibling_derived_constant` is discharged: the sibling was found, read, and rejected with a reason.

---

## 4. The production edit, as code

`internal/tls/config.go` — one file. Two new unexported functions plus one call line.

```go
// installALPNMismatchFallback makes a TCP downstream chain match reference Envoy
// on a NON-OVERLAPPING ALPN offer. The reference selects nothing and COMPLETES
// the handshake (BoringSSL alpn_select_cb -> SSL_TLSEXT_ERR_NOACK); crypto/tls
// instead alerts no_application_protocol (handshake_server.go negotiateALPN).
// Handing the stdlib a config with an EMPTY NextProtos takes its own
// "server advertises nothing" branch and reproduces the reference exactly.
//
// Installed ONLY here, at the single entry of NewDownstreamConfig, never in
// NewQUICDownstreamConfig: negotiateALPN's RFC 9001 Section 8.1 guard is itself
// predicated on len(serverProtos) != 0, so emptying NextProtos on the QUIC path
// would silently DISABLE a mandated rejection. See ADR-0317.
//
// The alternate config is cloned PER HANDSHAKE, not pre-built: at this anchor
// cfg.ClientCAs and cfg.ClientAuth are not yet assigned (installPool writes them
// below), so a build-time snapshot would serve a mismatched-ALPN client a config
// with NoClientCert — an authentication bypass on a require_client_certificate
// chain. ADR-0317 Context.
func installALPNMismatchFallback(cfg *stdtls.Config) {
	if cfg == nil || len(cfg.NextProtos) == 0 || cfg.GetConfigForClient != nil {
		return
	}
	cfg.GetConfigForClient = func(hi *stdtls.ClientHelloInfo) (*stdtls.Config, error) {
		if len(hi.SupportedProtos) == 0 || alpnOverlaps(cfg.NextProtos, hi.SupportedProtos) {
			return nil, nil // no-op: the stdlib's own paths are already correct
		}
		alt := cfg.Clone()
		alt.NextProtos = nil
		alt.GetConfigForClient = nil // never dispatched twice; nil'd so the invariant is local
		return alt, nil
	}
}

// alpnOverlaps reports whether any offered protocol exactly equals an advertised
// one. RFC 7301 protocol IDs are opaque octet sequences, so the comparison is
// exact and case-SENSITIVE — the same comparison crypto/tls negotiateALPN makes.
func alpnOverlaps(advertised, offered []string) bool {
	for _, a := range advertised {
		for _, o := range offered {
			if a == o {
				return true
			}
		}
	}
	return false
}
```

and the single call, inserted as the new `:57`:

```go
	installALPNMismatchFallback(cfg)
```

Notes binding the IMPL:

- **The three guards in the entry function are each separately testable** and each must get an NC (§12): `cfg == nil` (defensive; unreachable from `:53`, which errors instead), `len(cfg.NextProtos) == 0` (a chain with no `alpn_protocols` must be left with a nil callback — the observable is `cfg.GetConfigForClient == nil`), `cfg.GetConfigForClient != nil` (idempotence; this row is the only installer today, so the guard is a forward assertion).
- **The callback has no error return path.** §2.3 shows a non-nil error would send `alertInternalError`; there is nothing here that can fail.
- **`+0` stat surface.** No new metric name. `ssl.connection_error` and `ssl.handshake` are the phase-74/94 names, unmoved; what changes is WHICH of them a mismatch arm increments. `BEHAVIOR_CONTRACT.md`'s `### Stat surface` ledger (`:5071`, tail entry Phase 94 at `:5139`) gives `+0` phases NO chain entry — stated at `:5130` for phases 79/80 — so **this row adds no ledger row**, discharging §10 item 5's conditional.
- Nothing else in `internal/` changes. The four-outcome taxonomy (`manager.go:452-459`), `classifyHandshakeErr` (`:465-477`), `isTransportHandshakeErr` (`:500-506`) and the phase-94 `Inc` (`:1352-1359`) are all untouched: after the row a mismatched handshake returns nil, so it classifies `outcomeOK` and never reaches the switch.

---

## 5. Unit-test design

### 5.1 Flip `TestNewManager_LiveHandshake_ALPNNegotiationFailure_Aborts`

`internal/listener/tls_handshake_negative_test.go`, currently `121-171`.

- **Rename** to `TestNewManager_LiveHandshake_ALPNMismatch_CompletesWithNoProtocol`.
- **Rewrite the doc comment at `:113-120`**, which today asserts the false parity (*"matching reference Envoy, which alerts no_application_protocol on ALPN mismatch"*). The replacement states the MEASURED reference behaviour and cites ADR-0317.
- **Rewrite the helper doc comment at `:85-87`** (§0.6) — *"the server ENFORCES ALPN overlap"* becomes a statement that the chain advertises `NextProtos` and that a non-overlapping offer falls back.
- **Hoist the inline `stats.NewRegistry()` at `:130`** into a local `reg` so the counters are readable.
- **Invert the arm at `:162-170`.** The handshake must SUCCEED; assert `ConnectionState().NegotiatedProtocol == ""` by exact equality.
- **Assert the counters** after both arms drain: `ssl.connection_error == 0` and `ssl.handshake == 2` on `listener.<normalizeAddr(addr)>.…`, via `counterValue` + `awaitDrained` as `manager_test.go:5231-5233` does.
- **Keep and strengthen the control at `:149-159`.** It already asserts `NegotiatedProtocol == "http/1.1"` by exact equality — that is the over-firing control (if the fallback fired when it should not, this reads `""`). Keep it, and keep it BEFORE the mismatch arm so the `handshake == 2` arithmetic is unambiguous.
- **`t.Errorf`, not `t.Fatalf`, per property** (`reference_fatalf_makes_assertions_unreachable`) — except the setup failures at `:132`/`:138`/`:144`, which must stay fatal.

**The NC for this test is the tip itself**, measured at §0.9: unpatched, the renamed test must FAIL at the mismatch arm; patched, it must PASS. That is a real red-to-green transition, not a fresh green.

### 5.2 `TestServeConnection_SSLConnectionErrorCounter` — a SEVENTH row and a CONTROL row

`internal/listener/manager_test.go:5209-5246`. The table today is six single-cause rows over `startOneWayTLSListener` (`:4754-4779`), which builds its chain with `mkDownstreamTSInline` — **no ALPN**. An ALPN-capable listener helper does not exist.

- **Add `startOneWayTLSListenerALPN(t, pki, alpn []string) (*stats.Registry, string)`** beside `startOneWayTLSListener`, identical except it calls `mkDownstreamTSInlineALPN` (`tls_handshake_negative_test.go:88`, same package) with the given list. Same-package reuse; no export, no move.
- **The table's `dial` field is `func(t, addr)` and its listener is fixed**, so the two new rows cannot be expressed as table rows without changing the struct. **Specified: add a field `listen func(t *testing.T, pki handshakeTestPKI) (*stats.Registry, string)`, defaulting to `startOneWayTLSListener` when nil.** The six existing rows are untouched byte-for-byte except for the struct literal gaining no field. ⚠️ **If the IMPL finds the nil-default adds more noise than a separate table, a SECOND table function is acceptable — but then the "no other `ssl.*` leaf moved" loop at `:5239-5243` must be duplicated, not dropped.**

| # | name | listener ALPN | client offer | `connection_error` | other `ssl.*` |
|---|---|---|---|---|---|
| 7 | `alpn_mismatch_bogus` | `["h2","http/1.1"]` | `["bogus/9"]` | **0** | `handshake == 1`, rest 0 |
| 8 (control) | `alpn_http11_vs_h2_only` | `["h2"]` | `["http/1.1"]` | **0** | `handshake == 1`, rest 0 |

⚠️ **Row 8 is the discrimination guard, and it is GREEN AT THE TIP.** Go's `http11fallback` already serves that shape (§2.2). A "fix" that handles only it would leave row 7 red; a table containing only row 8 would prove nothing (`reference_coincidental_method_agreement`). **Both rows must be present, and the PLAN must record that row 7 is red at the tip and row 8 is green at the tip** — measure both before the fix, not after.

⚠️ The existing loop at `:5239-5243` asserts every OTHER `ssl.*` leaf is 0, including `handshake`. Rows 7 and 8 both expect `handshake == 1`, so the loop's leaf list must become per-row rather than fixed, or the two rows must carry their own expectation map. **Specified: give the row struct a `wantLeaves map[string]int64` merged over a zero default**, so every row keeps asserting which counters did NOT fire (`next-prompt.txt` §7g).

### 5.3 The mTLS-preservation arm — REQUIRED by §0.1, and absent from the BRAINSTORM

A unit test that proves the fallback does not weaken client authentication:

**`TestServeConnection_ALPNMismatch_PreservesClientAuth`** — a listener with `alpn_protocols: ["h2","http/1.1"]` AND `require_client_certificate: true`, driven by two arms on the same listener:

| arm | client cert | ALPN offer | expected |
|---|---|---|---|
| a | **valid** | `["bogus/9"]` | handshake COMPLETES, `NegotiatedProtocol == ""` |
| b | **withheld** | `["bogus/9"]` | handshake **REJECTED**, and it is the CERT error that fires, not the ALPN one |

Arm (b) is the one that fails under §0.1's pre-built-snapshot bug and passes under §4. Arm (a) is its over-firing partner: without it, arm (b) alone is satisfied by simply not implementing the row.

⚠️ **Assert WHICH error fired** (`next-prompt.txt` §7g): arm (b)'s failure must carry the client-certificate signature, and must NOT be `no_application_protocol`. ⚠️ **If a variant with an UNTRUSTED (rather than withheld) cert is added, the Go client must be forced to SEND it** — `reference_go_client_cert_withholding`: a Go client silently withholds a cert whose issuer is not in the server's `certificate_authorities`, making the arm vacuous.

### 5.4 Guard-level tests in `internal/tls`

Three small tests over `NewDownstreamConfig` output, asserting the guards of §4 at the seam rather than through a handshake:

- a context WITHOUT `alpn_protocols` ⇒ `dc.TLSConfig.GetConfigForClient == nil`;
- a context WITH `alpn_protocols` ⇒ `dc.TLSConfig.GetConfigForClient != nil` **and** `dc.TLSConfig.NextProtos` unchanged (the base config must never be emptied);
- the callback invoked directly with a `*stdtls.ClientHelloInfo` for each of: empty `SupportedProtos` ⇒ `(nil, nil)`; overlapping ⇒ `(nil, nil)`; non-overlapping ⇒ non-nil, `NextProtos == nil`, `GetConfigForClient == nil`, **and `ClientAuth`/`ClientCAs` equal to the base config's** — the direct unit expression of §0.1.

⚠️ These use a **production-representative** context built by the package's existing test helpers, not a hand-rolled `*stdtls.Config` (`next-prompt.txt` §7h: a table can carry an invented input no production path produces).

---

## 6. Fixture `0120-tls-connection-error` — arm (vi)

### 6.1 What is there now

`test/fixtures/0120-tls-connection-error/` — no `inputs/`, `driver/` + `pki/`, blank-imported at `test/differential/runner_test.go:147`, port **10126**, one `tcp_proxy` echo listener `l_conn_err` with `require_client_certificate: true` and a `tls_params` block. `driver/driver.go` (618 lines) sequences five arms in `driveSide` and scrapes `/stats/prometheus` in `AssertStats`, keyed on metric NAME only (`scrapeProm` strips labels — required, because `envoy_listener_address` diverges cross-side).

The pins at `driver.go:73-91`:

```
73  // ⚠️ THESE ARE ARM ARITHMETIC. Adding a sixth arm INVALIDATES them.
78  //   arm                       connection_error   handshake
79  //   (v) valid + client cert   +0                 +1
80  //   (i) bad version (TLS1.1)  +1                 +0
81  //   (ii) plaintext HTTP       +1                 +0
82  //   (iii) garbage bytes       +1                 +0
83  //   (iv) clean FIN, 0 bytes   +0                 +0
89  wantConnectionError = 3
90  wantHandshake       = 1
```

### 6.2 The change

- **Both YAMLs gain `alpn_protocols: ["h2", "http/1.1"]`** as the first key of `common_tls_context` — `envoy.yaml:68` and `envoy-go.yaml:57`. Same key, same value, same position on both sides; the `0004`/`0079`/`0080`/`0119` shape.
- **Arm (vi): valid client cert + ALPN offer `["bogus/9"]`**, appended LAST in `driveSide` so the five existing arms keep their order and their `record()` indices.
- **Expected after the fix, on BOTH sides:** arm (vi) contributes `connection_error +0`, `handshake +1`. ⇒ `wantConnectionError` STAYS **3**, `wantHandshake` **1 -> 2**.
- **At the tip, on the SUBJECT only:** arm (vi) contributes `+1 / +0`, so the fixture is RED at the tip — the deterministic cross-side gate this row adds. `BRAINSTORM.md` §7.1's finding stands: there is no existing gate.

⚠️ **EVERY FIGURE IN THE PRECEDING BULLET IS A PREDICTION.** `driver.go:73` says the pins are arm arithmetic and the BRAINSTORM says to re-measure. **The PLAN must boot BOTH sides and read all six arms per side from the live admin, then write the table from the reading** — including re-confirming that adding `alpn_protocols` leaves arms (i)-(v) unmoved. It is expected to (none of them offers ALPN), but "expected" is not a measurement.

- **`expectations.yaml`** `:63-72` (the ord table) gains a sixth row; `:118-119` gains the re-measured totals. **`README.md`** arm table at `:77-83` and pin block at `:182-187` likewise.
- **`+0` fixtures** (`0120` is EXTENDED, `0121` stays free), **+0 BackendKinds** (tail STAYS 38, `fixture.go:614`), **port unchanged (10126)**. The three registration gates (`reference_differential_fixture_three_registration_gates`) are untouched because no directory is added; the extractor of `next-prompt.txt` §3e must still reconcile **122 = 122, both `comm` directions empty**, and it does at this tip (98 `driver/` + 24 `inputs/`, NC'd in both directions).

### 6.3 Over-firing control

Driving arm (vi) TWICE must read `handshake +2`, `connection_error +0` — the fixture-level partner of §5.2 row 8. Whether it lands as a seventh arm or as a PLAN-time probe is the PLAN's call; if it lands, `wantHandshake` moves again and the table is re-measured again.

### 6.4 Why no other fixture can regress — proven, not asserted

Four other fixtures carry downstream `alpn_protocols` on TCP listeners (§0.5). **None can change behaviour under this row, and the proof needs no measurement:** at the tip the server ENFORCES overlap, so every one of those fixtures is green today only if its clients either overlap the advertised list or send no ALPN extension at all. Those are exactly the two cases §4's callback returns `(nil, nil)` on. `0104` carries `alpn_protocols: ["h3"]` on a QUIC listener and the install is TCP-only. **The PLAN still runs the full 122-fixture suite** — this argument bounds the risk, it does not replace the gate.

---

## 7. Comment and documentation reconciliation

The §3.4 roster, corrected and extended, set-differenced against the byte-untouched gates of §1. **Everything in this table lands at the IMPL.**

| site | what it says now | change | stage |
|---|---|---|---|
| `internal/tls/config.go:25` | `DownstreamConfig` is embedded in *"the listener's `GetConfigForClient` callback"* — describes a phase-03 design that no longer exists | rewrite: the chain's config is passed directly to `stdtls.Server`; this row installs the FIRST real `GetConfigForClient`, for ALPN fallback only | IMPL |
| `internal/tls/doc.go:5` | *"the SNI-wildcard match predicate used by the listener manager's `GetConfigForClient` callback"* | same correction | IMPL |
| `internal/listener/manager.go:138` | *"the legacy … listener-level `tlsCfg` (with the `GetConfigForClient` SNI dispatch callback) … are gone"* | **factually correct as history and must NOT be reverted**; append that a per-chain `GetConfigForClient` returns at phase 95 for ALPN fallback, which is not the phase-03 SNI dispatch | IMPL |
| `internal/tls/config.go:264` | `NewQUICDownstreamConfig` doc: *"so ALPN from `alpn_protocols` … are reused"* | not wrong, but now incomplete — add one clause: the phase-95 fallback is deliberately NOT installed here (§2.2) | IMPL |
| `internal/listener/tls_handshake_negative_test.go:85-87` | helper doc: *"the server ENFORCES ALPN overlap"* — **§0.6, missing from the roster** | rewrite | IMPL |
| `…tls_handshake_negative_test.go:113-120` | test doc: *"must ABORT … matching reference Envoy, which alerts `no_application_protocol`"* — FALSE | rewrite (§5.1) | IMPL |
| `…tls_handshake_negative_test.go:121-171` | `_Aborts`, `t.Fatal` at `:169` — pins the wrong side | rename + invert (§5.1) | IMPL |
| `internal/listener/manager_test.go:5209-5246` | six-row table | seventh + control rows (§5.2) | IMPL |
| `test/fixtures/0120-…/driver/driver.go:73-91` | five-arm arithmetic, `3` / `1` | six arms, RE-MEASURED (§6.2) | IMPL |
| `test/fixtures/0120-…/{envoy,envoy-go}.yaml` | no `alpn_protocols` | add on both sides (§6.2) | IMPL |
| `test/fixtures/0120-…/expectations.yaml:63-72, :118-119`; `README.md:77-83, :182-187` | five-arm tables and pins | six arms | IMPL |
| `BEHAVIOR_CONTRACT.md:1944` | *"the negotiated value … is not surfaced to the fixture driver in phase 03. If a later phase asserts ALPN negotiation, it adds a fixture opt-in and extends this subsection."* | **this row is that later phase** — extend WITHIN-LINE by literal text (§11) | **IMPL, not SPEC (§0.2)** |
| `DECISIONS.md:18862` (ADR-0316 §Consequences (vi)) | frames the ALPN arm as a coverage gap | **RETAINED VERBATIM** — DECISIONS.md is append-only; `ADR-0317` §Context corrects the framing | SPEC (this commit) |
| `ROADMAP.md:156`, `STATE.md:18` | *"cheapest follow-on … unit table"* | superseded by row 95's registration; **not edited** | none |
| memory `reference_driver_receiver_port_race_aborts_binary` | said FOURTEEN | corrected to ~36 at the BRAINSTORM close (memory, not repo) | done |

⚠️ **`BEHAVIOR_CONTRACT.md:1961`** is the ALPN-consumer paragraph (ADR-0050/ADR-0083, 736 chars). It describes codec selection on the negotiated value and needs **no edit**: after the row a mismatched connection presents `NegotiatedProtocol == ""` at the HCM boundary, which is byte-for-byte the state the already-passing no-ALPN arm produces, and `hcm/filter.go:104-106` dispatches `""` to the H1 driver — which is what the reference does (200 over HTTP/1.1). **The consumer side is unchanged, and the measurement that proves it is arm 2 of the BRAINSTORM's cross-side table, which already agrees.**

---

## 8. SDS interplay — RECORDED, not solved

`cfg.Certificates` is populated by the SDS provider at BUILD time (`config.go`'s validation-context switch; `reference_sds_fetchfail_posture_init_hold`: the posture is initial-fetch/init-hold, and there is no rotation path at this tip). The per-handshake `Clone()` of §3.2 copies the slice HEADER, so a clone observes whatever `cfg.Certificates` holds at handshake time — which is the correct behaviour if a future phase ever rotates in place, and identical to today's behaviour if it never does.

Two things are recorded and deliberately NOT solved here:

1. **If a future phase rotates `cfg` in place**, the base config and every future clone stay consistent automatically. This is the second reason §3.2 chose the clone over a snapshot; the first is §0.1.
2. **No fixture combines SDS with `alpn_protocols`** — `0103` and `0108`-`0111` carry SDS and no `alpn_protocols`; the five ALPN fixtures of §0.5 carry no SDS. **So this row ships no cross-side evidence for the combination, and says so rather than implying coverage.** Chartering an SDS+ALPN fixture is explicitly out of scope.

---

## 9. Gates

The row's IMPL runs the standing six-gate posture; **this SPEC runs none of them and names them** (`next-prompt.txt` §10):

| gate | expectation |
|---|---|
| (a) differential | **122/122**, `-count=1` NOT optional, fixture set asserted BY NAME in both directions |
| (b) non-Docker sweep | **235 packages**, gated on `PIPESTATUS[0]` + a set reconciliation |
| (c) h2spec | `95 tests, 94 passed, 1 skipped, 0 failed` |
| (d) fuzzers | **56 targets / 48 files** — unchanged, the row consumes no new config field |
| (e) anchored panic gate | `^panic:\|DATA RACE\|SIGSEGV` = **0**, and PROVEN LIVE |
| (f) `REVIEW.md` | absent — standing departure, NAMED not claimed |

⚠️ `-race` on the differential suite is VACUOUS (the subject is an unraced subprocess). ⚠️ `go test ./...` drives Docker; exclude BOTH drivers: `go list ./... | grep -vE '/test/differential$|/test/conformance/h2spec$'`.

**Live flake register the PLAN inherits** (`next-prompt.txt` §4 — the index is the memory dir, not this list): two SDS dial-budget flakes plus `TestSDSEndToEnd_FetchFailure_BootFailsClosed`; the driver-owned receiver port race (aborted a full run at the 94 IMPL); `internal/httpclient` zero-value; the two 84.2-era flakes; the reference h2spec section-8 flip; `TestOutlierDetector_ConcurrentEjectExactlyOnce`; `0061-lb-ring-hash`'s sigma margin; `TestP83_StopPauseTimer_IsAuthoritativeAgainstAnEnteredClosure`. ⚠️ `TestServerConn_TinyWindowDelivery` is NOT a flake — a recurrence is a REGRESSION of row 91. ⚠️ A green rerun clears nothing.

---

## 10. Counts, re-derived at THIS stage's own tip (`abc309d7`)

Discharges `BRAINSTORM.md` §10 item 9 and §6.4. Every figure below was RUN, not inherited.

| axis | at this tip | after this SPEC | after the ROW |
|---|---|---|---|
| `ROADMAP.md` lines / data rows (`want`) | **245 / 127** | unchanged | unchanged (row 95 FLIPS, no ADD) |
| sentinel check (1) | `NOT DONE: row 95`, one line | unchanged | SILENT |
| sentinel check (2) / (3) | **SIX** at `:205 :211 :217 :227 :233 :241` / SILENT | unchanged | unchanged |
| `DECISIONS.md` lines | **18872** | **+30**, matching the phase-94 SPEC's own `DECISIONS.md` numstat at `307f2e3d` | grows again at the IMPL, which APPENDS §Decision + §Consequences IN PLACE |
| `^---$` in `DECISIONS.md` | **216** | **STAYS 216** — the block adds none | 216 |
| `^## ADR-` / bare `^## ` | **315 / 323** | **316 / 324** | unchanged |
| `DECISIONS.md` tail / next-free ADR | **ADR-0316** / **`ADR-0317`** (TAIL-derived; `^## ADR-0317` reads 0) | **ADR-0317** / `ADR-0318` | unchanged |
| house `PROPOSED` guard `^> \*\*STATUS: PROPOSED` | **DISARMED** | **RE-ARMED by this block** | disarmed again at the IMPL |
| ADR-0231 decoy `^\*\*Status:\*\* PROPOSED` | **1**, at `:14866`, BYTE-UNTOUCHED | unchanged | unchanged |
| `BEHAVIOR_CONTRACT.md` lines | **5989** | **BYTE-UNTOUCHED (§0.2)** | +0 lines (WITHIN-LINE edit at `:1944`) |
| stat surface | *(DELTA only — three inconsistent absolutes are live)* | **+0** | **+0**, and **no ledger row** (`:5130` convention) |
| `STATE.md` / `STATE_HISTORY.md` lines | **65 / 548** | rolled in place / **+1** | — |
| archive guard strict / parenthetical / loose | **163 / 61 / 224** (`163 + 61 = 224` exactly) | **163 (DELTA 0) / 62 / 225** | — |
| phase dirs | **136** | unchanged | unchanged |
| fixtures | **122**, tail `0120`, `0121` FREE | unchanged | **122** — `0120` EXTENDED |
| fixture extractor | dirs **122** = imports **122**, both `comm` directions EMPTY, split **98 `driver/` + 24 `inputs/`** | unchanged | unchanged |
| BackendKind tail | **38** (`fixture.go:614`) | unchanged | **38** |
| fuzzers | **56 targets / 48 files** | unchanged | unchanged |
| `go.mod` require entries | **67** (⚠️ the `grep -cE '^\s+[a-z0-9./-]+ v[0-9]'` form reads **62**) | unchanged | unchanged |
| `go list ./...` | **237** (**235** excluding the two Docker drivers) | unchanged | unchanged |
| `-family row` | **96 occurrences / 68 lines** (`--` before the pattern) | unchanged | unchanged |

⚠️ **NO ABSOLUTE STAT-SURFACE FIGURE IS QUOTED ANYWHERE IN THIS DOCUMENT**, per ADR-0316 §Consequences (x): three mutually inconsistent absolutes are live in this tree at one tip.

⚠️ The malformed-row figure reconciles only under an ESCAPE-AWARE field count, and the escape-aware command is itself breakable: the correct form is `sed 's/\\|//g' F | awk -F'|' '/^\| *[0-9]/ && NF!=8'` with **NO file argument to awk** — passing the file makes awk ignore stdin and print the NAIVE figure under the escape-aware label.

---

## 11. The pinned edit map — WHICH STAGE LANDS WHAT

**This SPEC commit:** the five files of §1. Nothing under `internal/` or `test/`.

**The IMPL commit** lands everything in §7 marked IMPL, plus:

**`BEHAVIOR_CONTRACT.md:1944`, edited BY LITERAL TEXT, WITHIN-LINE.** The line is 357 characters (§0.4) and one paragraph. The clause to replace is exactly:

> `the negotiated value (which wins the ALPN negotiation) is not surfaced to the fixture driver in phase 03. If a later phase asserts ALPN negotiation, it adds a fixture opt-in and extends this subsection.`

The replacement must state, at minimum: (a) that phase 95 is the phase that promise named; (b) the MEASURED reference rule of §2.1, all three offer classes; (c) that the fixture opt-in is `0120` arm (vi), asserted cross-side on `ssl.handshake` / `ssl.connection_error`; (d) that the fallback is TCP-only and that QUIC's RFC 9001 §8.1 rejection is PARITY, not a departure. **No new departure is named — after the row there is none.**

⚠️ **Do not `sed` the line by number.** Anchor on the literal clause. ⚠️ **The edit must NOT touch `:1961`** (§7).

⚠️ **`ROADMAP.md` row 95 flips `in-progress -> done` at the IMPL and NOT before.** The row summary must carry NO unescaped `|` — count fields (want **8**) under BOTH the naive and the escape-aware form, BEFORE and AFTER installing.

---

## 12. Negative-control roster

⚠️ **NEUTRALISE, NEVER REVERT** — every NC must leave the package compiling. ⚠️ **NC every new pin, and NC the fixes too.** ⚠️ **An NC that leaves a control green is not evidence that control does work** — where noted, a SECOND isolating NC is required.

| # | pin | neutralisation | must read |
|---|---|---|---|
| 1 | §5.1 flipped test | **the tip itself** (no patch) | RED at the mismatch arm |
| 2 | §5.1 control arm (`NegotiatedProtocol == "http/1.1"`) | make the callback fire unconditionally (drop the overlap check) | RED — proves it catches OVER-firing |
| 3 | §5.2 row 7 | the tip itself | RED |
| 4 | §5.2 row 8 (control) | the tip itself | **GREEN at the tip** — this is the point; row 8 is NOT a red-flip pin |
| 5 | §5.2 row 8, isolating | implement the fallback for the `http/1.1`-vs-`h2` shape ONLY | row 8 GREEN, row 7 RED — proves the pair discriminates a stdlib-only "fix" |
| 6 | §5.2 `wantLeaves` map | set one row's map to the wrong leaf | RED — proves the which-did-NOT-fire assertion is live |
| 7 | §5.3 arm (b) | pre-build the alternate at `:57` instead of cloning per handshake (**§0.1's bug, written deliberately**) | RED — and this is the ONLY pin in the roster that catches it |
| 8 | §5.3 arm (a) | do not implement the row | RED — proves (b) is not satisfiable by omission |
| 9 | §5.4 guard `len(cfg.NextProtos) == 0` | remove the guard | RED — a no-ALPN chain must keep a nil callback |
| 10 | §5.4 base-config assertion | have the callback empty `cfg.NextProtos` in place instead of on a clone | RED — proves the base config is never mutated |
| 11 | §6.2 arm (vi) | the tip itself, subject side | RED (`+1 / +0` against a `+0 / +1` pin) |
| 12 | §6.3 over-firing control | drive arm (vi) twice | `handshake +2` |
| 13 | the `0120` YAML edit | remove `alpn_protocols` from ONE side only | RED cross-side — proves both YAMLs were edited |

⚠️ **NC 7 is the roster's most important cell and it is the one no inherited document asked for.** ⚠️ **A green test run is not evidence a site is exercised** — the PLAN must add a `panic()` reachability control inside the callback's mismatch branch and prove it fires (`next-prompt.txt` §7c). ⚠️ **`go test` without `-v` prints zero `=== RUN`**; `RUN=0` beside `RC=0` is a vacuous green. ⚠️ **A `-run` selector matching nothing prints `[no tests to run]` and EXITS 0.** ⚠️ On `-v` output use `grep -cE '^(FAIL|--- FAIL)\|^ *--- FAIL'`; an unanchored `grep -c 'FAIL'` reads nonzero on a fully green tree.

---

## 13. Sentinel, archive, and roll

### 13.1 The three checks — RUN MECHANICALLY AT THIS TIP, actual output

- **(1)** `want=127`: one line, `NOT DONE: row 95`.
- **(2)** **SIX** hits, at `:205 :211 :217 :227 :233 :241`.
- **(3)** SILENT.

⇒ **The sentinel does NOT fire. `stop` was evaluated and deliberately NOT created** (verified absent at the git root and in the stage worktree).

Per-line md5 of the six check-(2) windows, **trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`) — **all six byte-identical to the phase-95 BRAINSTORM close**:
`205 10d7807bf02d` · `211 4a92f7e62fc6` · `217 2a7eb298b9fd` · `227 242e53c6f7a3` · `233 b2680e6f4fbf` · `241 6caa1c3ce0e7`
⚠️ The digest is METHOD-SENSITIVE — these match ONLY with the trailing newline included.

### 13.2 The four NCs and the check-(2) positive control — ALL RUN, ALL FIRED

- **NC-A** (doctor row 62 to `in-progress`, `want=127`): `NC LANDED? [ in-progress ]` inspected first; check (1) then reads **TWO** lines — `NOT DONE: row 62`, `NOT DONE: row 95`.
- **NC-B** (denominator, `want=126`, real file): **TWO** lines — `NOT DONE: row 95` then `GATE FAIL: examined 127 data rows, expected 126`.
- **NC-C** (the mandatory check-(3) NC): residual **0**, and `NEVER OPENED: gRPC` FIRED.
- **NC-D** (`-family row` with `--`): **96** occurrences / **68** lines.
- **Check-(2) positive control**: residual **0** with **6** substitutions asserted — BOTH phrases substituted, because the longer phrase does not contain the shorter one as a substring and a one-phrase control reports a residual of 5 that reads like a finding.

⚠️ A SPEC moves none of these, and that is PROVEN above rather than assumed. ⚠️ NC shapes change across a row flip or add — never inherit one.

### 13.3 The archive guard and the eviction

Strict **163** / parenthetical **61** / loose **224**, and `163 + 61 = 224` exactly, under the anchored-occurrence forms. **This close appends in the PARENTHETICAL form, so the STRICT guard moves by DELTA 0** — that is the guard's whole content; the number is a SUBSET, not the entry count. ⚠️ **This stage's archive line names NO positive-control figure** — the archive's controls are self-incrementing.

**The evictee is the phase-93 IMPL entry, chosen by LIST POSITION, not by date (§0.11).** The date read at this tip is `09-05, 09-03, 09-02, 09-01, 09-01` — tied at the oldest position, so the date cannot discriminate. After this close the tie is broken (one 09-01 entry remains) and §Recent reads `95 BRAINSTORM (09-06), 94 IMPL (09-05), 94 PLAN (09-03), 94 SPEC (09-02), 94 BRAINSTORM (09-01)`.

**The eviction check is a PAIR, not a single count (§0.10).** For the evictee's label: on `STATE.md` **1 -> 0**, on `STATE_HISTORY.md` **0 -> 1**. Both halves must be measured; the bare strict and naive forms (5 and 6 at this tip) do not answer the question.

⚠️ **The §Recent preamble sentence rolls too, and must NOT spell the evictee's label** — a preamble that names it makes the eviction check match its own prose.

---

## 14. What this SPEC does not decide

- **Any absolute stat-surface figure.** Delta only; the row is `+0`.
- **The other TEN fixed `ssl.*` names and FOUR dynamic families** — still blocked on NAMING (the stat-name charset bans the hyphen).
- **An SDS + `alpn_protocols` fixture** (§8) — recorded as absent coverage, not chartered.
- **The QUIC path.** Untouched, and §2.2 shows touching it would remove a mandated rejection.
- **`codec_type: HTTP2` (non-AUTO) with a mismatched offer** — unmeasured on both sides (`BRAINSTORM.md` §2.4), out of scope.
- **The upstream side.** `internal/cluster/manager.go:870-877` enforces `alpn_protocols` containing `h2` for h2 clusters; that is a config-validation rule, not a negotiation fallback, and is untouched.
- **The driver-owned receiver port race** (~36 driver files) stays BANKED — the most defensible next pick if a full differential run aborts again.

---

## 15. What the PLAN owes

1. **A task count DERIVED, not carried.** No figure is quoted here. The BOOTSTRAP §6.1 split-gate trigger is ~25 tasks OR ~1500 net lines — check it against the derived count and split if it trips.
2. **BOOT BOTH SIDES FIRST and RE-MEASURE all six `0120` arms per side** (§6.2) before writing a single pin. The arm arithmetic is a prediction until then, and `driver.go:73` says so itself.
3. **Run the package sweep, not a selector** (§0.8) — at minimum `./internal/tls/` and `./internal/listener/`, `-count=1 -v`, `RUN` asserted nonzero.
4. **Record row 7 RED and row 8 GREEN at the tip BEFORE the fix** (§5.2). Measuring them only after the fix cannot distinguish the row from the stdlib.
5. **Land NC 7** (§12) — write §0.1's pre-build bug deliberately, prove §5.3 arm (b) reddens, then revert it under a `sha256sum -c` guard.
6. **Prove the callback's mismatch branch is REACHED** with a `panic()` reachability control, not with a green run.
7. **Keep the `BEHAVIOR_CONTRACT.md:1944` edit atomic with the code** — it is a governing document; a contract that describes behaviour the tree does not yet have is worse than one that is merely stale.
8. **Assert the fixture set BY NAME, both directions, against the fixtures on disk**, and re-run the §3e extractor NC. The full suite takes ~400s and `-count=1` is not optional.
9. **Re-derive every count in §10 at the PLAN's own tip.** ⚠️ `feedback_brief_citations_not_evidence` applies to every number in this document.
