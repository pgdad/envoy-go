# Phase 03 — TLS Review

**Reviewer:** superpowers:code-reviewer subagent
**Date:** 2026-04-24
**Review range:** `3559a2e` (phase-02 COMPLETE) → `aff6fbf` (phase-03 impl tip)
**Verdict:** APPROVED WITH FOLLOW-UPS

---

## Executive summary

Phase 03 lands envoy-go's first cryptographic surface: a clean `internal/tls/` package built on stdlib `crypto/tls` (aliased `stdtls`), a multi-chain SNI-routing listener, a transport-agnostic `Cluster.Dial(ctx)` upstream dialer, a TLS-aware `halfClose`, a deterministic ECDSA-P256 PKI tree under `test/fixtures/0002-tls-tcp/pki/`, a `FuzzTLSContextParse` fuzz target, eight new ADRs (0029–0036), and a new BEHAVIOR_CONTRACT TLS subsection. The 15-task PLAN was executed cleanly: every task entry in PROGRESS carries a non-empty `**Commits:**` line, every cited SHA resolves in `git log`, ADRs are sequentially numbered without gaps, and `go vet ./...` / `golangci-lint run` / `go test -count=1 -timeout=10m ./...` all exit zero from a fresh session. PKI determinism re-verifies: a fresh `go run ./pki/gen` from the impl worktree leaves the working tree clean.

The major caveat is **ADR-0035** — fixture `0002-tls-tcp` was narrowed mid-phase from "downstream TLS + SNI + upstream TLS" to "downstream TLS + SNI only" because the harness has plain-TCP backends and adding upstream-TLS would have required a harness extension that the PLAN forbade. Upstream TLS is fully implemented in `internal/cluster/cluster.go` (`Dial` TLS branch, `HandshakeContext(ctx)`) and exercised by 4 unit tests in `cluster_test.go` plus the `FuzzTLSContextParse` upstream seed, but it is NOT differentially asserted. ADR-0035 is honest about this and the BEHAVIOR_CONTRACT TLS subsection labels the rule "unit-tested only, not differentially asserted." The rationale is sound for phase 03 (the contradiction was structural, not negotiable, and the runtime is correct), but the phase's title — "TLS (Downstream Termination + Upstream Origination + SNI)" — overpromises relative to what the differential gate actually proves. This is the single most important consequence of the phase to surface in the next phase's planning: HTTPS in phase 04+ should drive upstream-TLS through a naturally-TLS fixture (or extend the harness with TLS-capable backends) so the `Cluster.Dial` TLS path gets cross-proxy validation.

Beyond the scope-narrowing observation, the implementation is well-structured (each new file is small, focused, and well-documented), the test coverage is generous (87.4 % in `internal/tls/`, 26+ subtests for parse paths, 15 new listener tests), and the error-prefix discipline is largely held. I recommend **APPROVED WITH FOLLOW-UPS** because the deviations are documented and the runtime is correct, but a small number of minor issues should be addressed in the first commit of phase 04 (or as a quick cleanup commit before phase-04 work begins).

---

## Verification of reviewer checklist (STATE.md 7-point)

### (1) PLAN's 15-task → commit mapping in PROGRESS is complete and SHAs resolve — PASS

I resolved every SHA cited in PROGRESS against `git log`:

```
$ for sha in 8f48101 a833d23 f63119e 71b4972 85ceb0b 38ee5f9 66af08e 926d93a \
              e252dbe 1c7dc31 e20ecc2 91bc8fa 9b5baa4 6ec3d0b ddbe63e d9f29a9 \
              71068a2 aff6fbf; do
    git rev-parse --verify --short=7 "$sha" >/dev/null && echo "OK: $sha" || echo "MISSING: $sha"
done
```

All 18 SHAs resolved (the impl tip `aff6fbf`, the verification commit `71068a2`, plus one or two SHAs per Task 1–15). No `**Commits:**` line in PROGRESS is empty. Task 11 SHA was rewritten once (`715fa7e` → `96f5bbb`) but PROGRESS records both. Task 12 / Task 11 ordering in PROGRESS is reversed relative to the implementation order on the branch (Task 12's SHA `91bc8fa` precedes Task 11's `e20ecc2` in the git log), but the PLAN explicitly allows interleaving and PROGRESS notes the ordering at each entry — not a finding.

### (2) ADR-0029..0036 are coherent, sequentially numbered without gaps, and cited in code or PROGRESS — PASS

`grep '^## ADR-'` confirms ADR-0029 → ADR-0030 → ADR-0031 → ADR-0032 → ADR-0033 → ADR-0034 → ADR-0035 → ADR-0036 are present in DECISIONS.md in file order, all dated `2026-04-24`, all `Status: Accepted`, all `Doctrine: D-3.5` except ADR-0034 (`D-3.6`) and ADR-0036 (`D-3.5, D-3.3`). Each ADR is cited in code or PROGRESS:

| ADR    | Citation site                                                                          |
|--------|----------------------------------------------------------------------------------------|
| 0029   | `internal/tls/datasource.go` head comment; PROGRESS Task 3                              |
| 0030   | `internal/tls/params.go` head comment; PROGRESS Task 4 (and inline `mapTLSVersion` doc) |
| 0031   | PROGRESS Task 5 commit message; ADR head comment cites stdlib stack                     |
| 0032   | `internal/cluster/cluster.go` `Dial` doc; PROGRESS Task 9; commit `e252dbe`             |
| 0033   | `internal/listener/manager.go` lines 80, 127, 184; PROGRESS Task 10; commit `1c7dc31`   |
| 0034   | `test/differential/fixture/fixture.go` doc; PROGRESS Task 12; commit `91bc8fa`          |
| 0035   | `test/fixtures/0002-tls-tcp/driver/driver.go` head comment; PROGRESS Task 13            |
| 0036   | `docs/envoy-go/BEHAVIOR_CONTRACT.md` line 177; PROGRESS Task 14                         |

**Supersedes header on ADR-0033** — verified to read `**Supersedes: ADR-0025**` on line 1001 of DECISIONS.md. The unusual punctuation (colon inside the bold span rather than after it, as ADR-0021 / ADR-0026 use) is harmless but inconsistent with the project's prior pattern. NOTE.

**ADR-0034 informal supersession** — verified to read `**Supersedes (informal):** the phase-02 fixture.Driver.Drive(ctx, refAddr, subjAddr) interface method codified in test/differential/fixture/fixture.go. No prior formal ADR — hence the (informal) qualifier.` This is fully compliant with the SPEC §4.4 requirement.

**ADR-0035 numbering shift** — ADR-0035 documents that PLAN.md originally assigned 0035 to Task 14's BEHAVIOR_CONTRACT subsection. Because ADR-0035 (this scope-reduction ADR) had to land before Task 14 in chronological execution order, it took the next sequential number, and Task 14's ADR shifted to 0036. The shift is recorded in ADR-0035's own Consequences block. Acceptable; the scheme is internally consistent.

### (3) SPEC §3 gates are evidence-backed — PASS (re-verified)

I re-ran every in-scope gate from the impl worktree on a fresh shell. Quoting verbatim:

```
$ go vet ./...
(empty — clean, exit 0)

$ go test -count=1 -timeout=10m ./internal/... ./test/helpers ./test/fixtures/...
?   	github.com/esalaine/envoy-go/internal/accesslog	[no test files]
ok  	github.com/esalaine/envoy-go/internal/admin	0.039s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.006s
ok  	github.com/esalaine/envoy-go/internal/cluster	0.007s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.007s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	0.007s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
?   	github.com/esalaine/envoy-go/internal/stats	[no test files]
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	0.015s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	0.004s
?   	github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	0.002s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	0.002s
?   	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/pki/gen	[no test files]
```

Differential suite (`./test/differential/`) requires the reference Envoy container; I did not re-run it because PROGRESS commit `71068a2` already quotes a fresh-session `--- PASS: TestDifferential/0002-tls-tcp (1.27s)` plus the other two fixtures verbatim, and the verification commit was authored in this lifecycle-state cycle (2026-04-24). The non-container portion of the test surface is reproduced clean above.

The PROGRESS verification block's quoted outputs are consistent with my live re-run on the same package set and machine timing (sub-100ms unit-test packages; `cmd/envoy-go` package at 0.563s in PROGRESS vs cached on this re-run). The block is not a synthetic transcript.

### (4) BEHAVIOR_CONTRACT.md TLS subsection is consistent with implementation — PASS

I cross-read the TLS subsection (lines 175–210) against the implementation:

- **"Plaintext-after-decryption byte equivalence"** (asserted) — fixture 0002 `driveSide` calls `helpers.TLSRoundTrip` and `runFixture` byte-compares the concatenated returns. Matches.
- **"Per-SNI chain-selection equivalence"** (asserted, witnessed via distribution) — `AssertDistribution` checks `[3,3,3]/[3,3,3]` per-cluster per-side. Implementation in `internal/listener/manager.go` `dispatch` re-runs `MatchServerName` post-handshake on `tlsConn.ConnectionState().ServerName`. Matches.
- **"Server-certificate identity by SNI"** (asserted, semantic only) — accurately characterizes `GetConfigForClient` returning the chain-specific `*stdtls.Config`. Matches.
- **"Upstream SNI + CA equivalence (unit-tested only, not differentially asserted)"** — explicitly downgraded per ADR-0035. The subsection is honest about the gap. Matches.
- **"Encrypted-side byte equivalence … not asserted"** — accurate; no harness code compares encrypted bytes.
- **"Negotiated ALPN value … not asserted"** — accurate; `commonTLSContextToConfig` populates `cfg.NextProtos = c.GetAlpnProtocols()` symmetrically on both sides but the negotiated value is not surfaced.
- **Parameter mapping caveats** — accurate against `params.go` (TLS-1.3 cipher names: `tls13CipherByName != 0` → diagnostic + drop; `signature_algorithms`: errors per `applyTLSParams` final block).

The subsection is well-written and accurate.

### (5) ADR-0035's narrowed differential scope is reviewer-acceptable — APPROVED WITH FOLLOW-UPS

ADR-0035 documents that fixture 0002 covers downstream TLS + SNI only; upstream TLS origination is unit-tested. The rationale (harness has plain-TCP backends; PLAN explicitly forbade harness changes; the two requirements are structurally contradictory) is sound and well-argued. The three options in §Rationale are evaluated honestly; option 1 (land + ADR + unit-test) is the right choice given the constraints.

What I want to flag for the next phase planner:

1. **The phase title "TLS (Downstream Termination + Upstream Origination + SNI)" overpromises** relative to what the differential gate proves. The implementation delivers all three; the *equivalence assertion* covers two of three. ROADMAP.md's phase-03 row should either be amended to reflect this or the next phase's SPEC should explicitly inherit the gap as a known follow-up.
2. **Unit-test adequacy for the upstream-TLS gap is acceptable but not lush.** `cluster_test.go` has 4 `Dial`-related tests (`Plaintext`, `TLS`, `TLS_HandshakeFailure`, `CtxCanceled`); `config_test.go` has 3 `Happy` + 8 `Error` upstream subtests. `FuzzTLSContextParse` exercises both downstream and upstream parsers from the same fuzz body. There is no test asserting that two `Cluster.Dial(ctx)` calls against the same TLS upstream send the *same* SNI string — this is implicit in the `*stdtls.Config.ServerName` being set once in `NewUpstreamConfig` and `Dial` not mutating it, but a one-line test would close the loop. SUGGESTION (not a blocker).
3. **The HTTPS phase is the natural place to close this gap.** Phase 04 (HTTP/1.1) or phase 05 (HTTP/2) will need TLS-capable upstream tooling regardless; that work also retires the fixture-0002 differential gap.

The unit coverage is adequate for phase-03 acceptance.

### (6) Fixture 0002 PKI is deterministically reproducible — PASS (re-verified)

Re-ran the PKI generator from the impl worktree:

```
$ cd test/fixtures/0002-tls-tcp && go run ./pki/gen
ok: 9 PEMs written to pki

$ git diff --exit-code pki/
(empty — exit 0)
```

The PKI tree is byte-identical to the committed copy. The Go-1.26 entropy-injection workaround (PROGRESS Task 7 documents `crypto/internal/rand.CustomReader` silently replacing custom readers; mitigated by going through `ecdh.P256().NewPrivateKey(scalar[:])` directly with a per-tag ChaCha8 stream) is non-obvious but correct, and the head comment on `pki/gen/main.go` should read by a future maintainer as intentional.

### (7) No `testdata/fuzz/` seed-corpus pollution — PASS

```
$ git status
On branch phase/03-tls-impl
nothing to commit, working tree clean
```

The verification block's three 30-second fuzz runs (PROGRESS commit `71068a2`) did not promote any seed entries. `find . -path './testdata/fuzz' -type d` (implicit via the clean status) confirms no `testdata/fuzz/` directory exists in any phase-03 fuzz package. Per ADR-0018's discipline, this is the expected outcome of a clean run.

---

## Findings

### Critical

None. The phase is functionally correct and the gates are green.

### Important

**I-1. `TLS_AUTO` mapping: doc-comment contradicts code.** In `internal/tls/params.go` lines 64–73, `mapTLSVersion(TLS_AUTO)` returns `(0, nil)` (i.e., no error, treat as unset). But the head comment on the same case reads:

> we cannot distinguish "unset" from "TLS_AUTO explicitly chosen" at the proto level, so we adopt the strict interpretation: explicit TLS_AUTO errors.

The "errors" half is wrong — the very next line returns `(0, nil)`. ADR-0030's mapping table explicitly says "TLS_AUTO → no-op (treat as unset)" and the test `TLS_AUTO min no-op` (params_test.go:29) asserts the no-op behaviour. The comment is a leftover from an earlier draft. This is not a correctness bug in the code, but it actively misleads anyone reading the file. SPEC §5.5's table also disagrees (says "TLS_AUTO → error") — see I-3.

**Fix:** Change the comment to match the code and ADR-0030 (one-line edit). Optionally tighten by deleting the case entirely (Go's switch on enum already covers all defined values; falling through to `default` would print a confusing "unknown tls protocol enum 0" error which is worse, so an explicit case with a clear comment is right — just fix the comment).

**I-2. Listener `GetConfigForClient` error string lacks `listener:` prefix.** `internal/listener/manager.go` line 302: `return nil, fmt.Errorf("listener %q: no filter_chain matches SNI %q", rt.name, sni)`. The phase's single-rule discipline ("every error crossing a package boundary begins with `<package>: `") is violated here: `crypto/tls` calls into our callback and propagates the error out of the package boundary back to its own handshake machinery, which logs it and aborts the connection. The error text starts with `listener ` (no colon, no space) and `crypto/tls` typically wraps it as `tls: handshake failed: ...listener "x": no filter_chain matches SNI "y"`, so a grep for `^listener:` won't find it.

The same shape recurs at line 319 (`"listener %q: post-handshake dispatch: ..."`) inside `dispatch`, but that one is a `log.Printf`, not a returned error — it goes to operator logs, not callers, so the discipline arguably doesn't apply.

**Fix:** Change to `fmt.Errorf("listener: %q: no filter_chain matches SNI %q", rt.name, sni)` (and same at line 319 if you want consistent log lines). Single-character fix per site.

**I-3. SPEC §5.5 table vs code disagreement on `TLS_AUTO`.** SPEC §5.5 line 223 reads:

> `TLSv1_2`/`TLSv1_3` → `stdtls.VersionTLS12/TLS13`. `TLSv1_0`, `TLSv1_1`, `TLS_AUTO` → error.

Code and ADR-0030 both say `TLS_AUTO` is a no-op. The test was deliberately updated mid-phase (PROGRESS Task 4 records: "TLS_AUTO test adjusted from the PLAN draft's 'TLS_AUTO min errors' to 'TLS_AUTO min no-op'"). ADR-0030 is the authoritative settlement; SPEC §5.5 is stale.

**Fix:** Either (a) amend SPEC §5.5 to match ADR-0030, or (b) cite ADR-0030 next to the relevant table row as the authoritative override. Phase plans are conventionally frozen at landing — but SPEC clearly documents the intended behaviour and is referenced by future readers. A one-line edit that reads "TLS_AUTO → no-op (treat as unset, per ADR-0030)" would close this. Could be addressed in the next-phase first commit.

**I-4. `fixture 0002` driver — orphaned `prefix` variable.** `test/fixtures/0002-tls-tcp/driver/driver.go` lines 103–112: `prefix := strings.TrimSuffix(strings.TrimSuffix(sni, ".envoy-go.test"), "alpha")` is computed and immediately silenced via `_ = prefix`. The variable is never used. The `statPrefix` value used in the YAML is computed by the `func() string{...}()` immediately below. This is dead code and the linter is missing it because of the `_ =`.

**Fix:** Delete lines 103–112 (`prefix := ...` and `_ = prefix`). The `statPrefix` IIFE is the only thing actually consumed in the YAML template and is fine on its own.

### Minor

**M-1. ADR-0033 `Supersedes:` header punctuation drift.** Reads `**Supersedes: ADR-0025**` (colon inside the bold span). Project prior practice puts the colon outside (e.g., ADR-0021: `**Supersedes:** ADR-0007` on line 559 of DECISIONS.md, and ADR-0026's similar `**Supersedes:** (informal) phase-00 sentinel contract...`). Mechanical inconsistency, harmless to readers.

**M-2. Listener `Stop()` / `Listeners()` race on `rt.netLn`.** `Stop()` holds `m.startedMu` and writes `rt.netLn = nil`; `Listeners()` reads `rt.netLn` without locking. This is harmless in practice — the bootstrap-test harness calls `Listeners()` only between `Start()` and `Stop()`, never concurrently with `Stop()` — but `go test -race` would flag it the moment a future test holds a reference and races. The `Manager.startedMu` exists already; extending its scope to `Listeners()` is one line.

**M-3. `chainSpecificityRank` initial `rank := 4` is unreachable.** Lines 232–248 of `manager.go`. Every `p` in `patterns` matches one of `*`, `*.`-prefixed, or `default` (which returns 0 immediately). So `rank` is always either 0, 1, or 2 by loop end — never 4, and never 3 (catch-all is handled by the `len(patterns) == 0` short-circuit above). The `rank := 4` is a "shouldn't happen but safe" sentinel. Acceptable defensive coding; could simplify to `var rank int = 2` (universal-wildcard default) for tighter readability.

**M-4. `internal/cluster/cluster.go:14` comment cites "SPEC §10 #2 settled" but the relevant SPEC item for `defaultConnectTimeout` is phase-02 SPEC §x, not phase-03 SPEC §10 #2.** Phase-03 SPEC §10 #2 is "Chain selection propagation from callback to filter dispatch" — wholly unrelated. The comment was probably meant to cite phase-02 SPEC. Trivial doc nit; doesn't affect behaviour.

**M-5. `internal/cluster/manager.go:82` and `:86` error texts say "phase 02 supports only STATIC" and "phase 02 supports only ROUND_ROBIN" though we are now in phase 03. Same for `:89` and `:140`. The cluster manager is unchanged from phase 02 in those code paths, which is why the strings stuck — but the messages mislead an operator into thinking they're running an older binary. Carry-over nit.

**M-6. Phase-02 REVIEW Minor 5 (`readyListenerAddrs` goroutine leak) explicitly deferred — confirmed.** SPEC §12 records this. Phase 03 does not touch the ready-sentinel path. Carrying forward to a future cleanup commit.

**M-7. Phase-02 REVIEW Minor 7 (prose-heavy `expectations.yaml`) deferred to phase 06/08 per ADR-0019 — confirmed.** Fixture 0002's `expectations.yaml` (23 lines) follows the phase-02 prose convention. Acceptable.

**M-8. `inlineString` indent helper is a clever but fragile one-off.** `test/fixtures/0002-tls-tcp/driver/driver.go:77-87`. The `keyIndent` parameter is hard-coded as a 22-space string at the call site (line 115). PROGRESS Task 13 documents the YAML-indentation iteration that arrived at this design. The helper works but the call sites bake in the indent depth as a string literal rather than a derived constant — any future restructure of the YAML template will need to recount spaces. This is exactly the sort of thing that earns a comment like "DO NOT reformat the surrounding YAML without recounting indents." A one-line caveat above `inlineString` would help.

---

## Strengths

- **Clean ADR discipline.** Each ADR is single-concern (ADR-0029 = DataSource, ADR-0030 = parameter mapping, ADR-0031 = stack, ADR-0032 = dialer, ADR-0033 = filter-chain subset, ADR-0034 = driver split, ADR-0035 = scope reduction, ADR-0036 = BEHAVIOR_CONTRACT). No ADR-0028-style bundling. Phase-02 REVIEW Minor 2 was avoided by construction.
- **`internal/tls/` package is small and well-factored.** Five files (sni.go, datasource.go, params.go, config.go, doc.go) plus tests; each has a clear responsibility; the `stdtls = crypto/tls` aliasing pattern is consistent everywhere.
- **TDD discipline visible.** Multiple PROGRESS entries record red→green transitions (Task 8: `# compile error: TLSRoundTrip undefined`; Task 9: TLS-extension test failures before `Cluster.Dial` TLS branch landed; Task 11: `got "", want "hello"` from filter not actually doing TLS). Those red-state captures are how you prove tests don't trivially pass.
- **PKI determinism work-around.** Task 7 surfaces a real Go 1.26 cryptographic-determinism gotcha (`crypto/internal/rand.CustomReader` silently replacing custom readers) and routes around it via `ecdh.P256().NewPrivateKey(scalar[:])`. That's a non-obvious failure mode the team will be glad they captured.
- **PROGRESS verification block is rigorous.** Commit `71068a2` quotes every gate verbatim (build, vet, lint, test, three 30s fuzz runs, PKI determinism, post-run `git status`). Reviewer can cross-check everything without re-running.
- **Phase-02 REVIEW carryover triage executed.** Minor 4 (`ctx` consumed via `Cluster.Dial`), Minor 6 (`Drive` split via ADR-0034), Minor 8 (BEHAVIOR_CONTRACT cross-reference) all visibly resolved in code/docs. Minors 5 / 7 explicitly deferred with rationale.
- **`halfClose` extension is a one-liner exactly as SPEC §5.6 promised.** Test coverage for `*stdtls.Conn.CloseWrite` exists (`TestFilter_Handle_HalfCloseOverTLS`).
- **Fuzz seed corpus matches SPEC §4.1.** Four seeds: well-formed downstream, well-formed upstream, truncated Any (with the PROGRESS-noted fix to ensure non-empty marshal), wrong type_url. Fuzz body asserts both no-panic and `tls: ` error prefix per SPEC.

---

## Recommendation

**APPROVED WITH FOLLOW-UPS.** The phase is correct, well-tested, and well-documented. No Critical findings. Four Important findings are minor enough to ship as a single follow-up commit (or as the first commit of phase 04) without re-entering BOOTSTRAP §5 step 3:

1. **I-1**: Fix the contradictory `mapTLSVersion(TLS_AUTO)` comment in `internal/tls/params.go`.
2. **I-2**: Add `listener: ` prefix to the `GetConfigForClient` error string in `internal/listener/manager.go:302`.
3. **I-3**: Reconcile SPEC §5.5's `TLS_AUTO` row with ADR-0030 (one-line clarifying edit).
4. **I-4**: Delete the orphaned `prefix` variable in `test/fixtures/0002-tls-tcp/driver/driver.go:103-112`.

Eight Minor findings (M-1 through M-8) carry forward to future-phase REVIEW triage in the same way phase-02 REVIEW Minors 5/7 carried into phase 03.

The single most important context to surface to the phase-04 planner: **fixture 0002 does not differentially exercise upstream TLS** (per ADR-0035). Phase 04 (HTTPS) or phase 05 (HTTP/2 + TLS) should close this loop, either by extending `test/differential/harness.go` to spawn TLS-capable backends or by introducing a naturally-TLS HTTPS fixture whose upstream dial validates `Cluster.Dial`'s TLS branch cross-proxy.

Phase 03 is ready to advance to lifecycle-state 6.
