# Phase 04 — HTTP/1.1 Review

**Reviewer:** superpowers:code-reviewer subagent
**Date:** 2026-04-25
**Review range:** `230fef6` (phase-03 COMPLETE / master HEAD before phase 04) → `bbe298f` (phase-04 impl tip; HEAD of `phase/04-http-1.1-impl` and master)
**Verdict:** APPROVED WITH FOLLOW-UPS

---

## Executive summary

Phase 04 lands envoy-go's first HTTP-aware dataplane: a clean `internal/filter/hcm/` package built atop the stdlib `net/http` parser/serializer surface, an HCM type-URL registration in the listener-manager, blank-imports in `internal/bootstrap/` for protojson round-trip of the new typed_configs, an `HCM smoke` variant in `cmd/envoy-go/main_test.go`, two new test helpers (`HTTPRoundTrip`, `HTTPHeaderDiff` + `PhaseFourHTTPAllowList`), an `HTTPExpectations` orchestration extension and an HTTP-echo backend kind in the differential runner, the new fixture `0003-http11-routing` (HCM + path/prefix routing + router + direct_response, with reference STRICT_DNS / subject STATIC per ADR-0027), eight new ADRs (0037–0044), a fuzz target `FuzzHCMConfigParse`, and a new BEHAVIOR_CONTRACT HTTP/1.1 subsection.

The 17-task PLAN executed cleanly: every PROGRESS entry carries a non-empty `**Commits:**` line, every cited SHA resolves in `git log --oneline 230fef6..bbe298f` (17/17 verified), ADRs are sequentially numbered without gaps (ADR-0037 → ADR-0044), each ADR is cited in code or PROGRESS, and on a fresh-session re-run from this worktree `go build / vet / golangci-lint run / go test ./... / go test ./test/differential -v` plus four 30-second fuzz runs all exit clean with a clean working tree. The differential gate is green on all four fixtures (`0000`/`0001`/`0002`/`0003`).

The phase-03 follow-ups landed before phase-04 began (commits `98cc35b` / `cbfe275`); the phase-03 REVIEW Minor list (M-1..M-8) was triaged in SPEC §12 with rationale per item — most deferred, two opportunistically-resolvable. **M-2** (Listener `Stop()/Listeners()` race) carries forward unaddressed (correctly: phase 04 does not touch the listener-manager lock surface). **M-5** (`internal/cluster/manager.go` "phase 02" error texts on lines 83/86/89/140) also carries forward unchanged because phase 04 does not touch `cluster/manager.go`. Neither is a phase-04 blocker.

The implementation surface is well-factored (12 files in `internal/filter/hcm/`, average 80 lines each), test coverage is generous (190+ subtests across the new package and helpers, including fuzz), and the error-prefix discipline (`hcm: `) is held throughout. ADR alignment is clean — all eight ADRs match the in-tree code with one minor caveat noted under Important findings (I-1).

I recommend **APPROVED WITH FOLLOW-UPS**: zero Critical findings, four Important findings (none of them blockers), and seven Minor findings. None requires re-entering BOOTSTRAP §5 step 3; all four Importants can ship as a single follow-up commit (or as the first commit of phase 05).

---

## Verification of reviewer checklist (BOOTSTRAP §5.5 / SPEC §3 gates)

### (1) PLAN's 17-task → commit mapping in PROGRESS is complete and SHAs resolve — PASS

I resolved every SHA cited in PROGRESS against `git log`:

```
$ for sha in ae52f36 c33d3c8 dcc6b40 d57ae8b 95ea7e8 7359397 ab6dc50 c308cb8 \
              6857383 951c90f ab4520f d81b38b 5bcce5f 0acb263 1a87738 56b29a8 \
              dc079c2; do
    git rev-parse --verify --short=7 "$sha" >/dev/null && echo "OK:  $sha" || echo "MISS: $sha"
done
```

All 17 SHAs resolved. No `**Commits:**` line in PROGRESS is empty. Tasks landed in the order PLAN specified; each task commit is followed by a small `PROGRESS SHA-fill` text-only commit (the project's standard double-commit pattern from phases 01–03).

### (2) ADR-0037..0044 are coherent, sequentially numbered without gaps, and cited in code or PROGRESS — PASS

`grep '^## ADR-'` confirms ADR-0037 → ADR-0044 are present in DECISIONS.md in file order, all dated `2026-04-25`, all `Status: Accepted`. Each ADR is cited in code or PROGRESS:

| ADR    | Citation site                                                                 |
|--------|-------------------------------------------------------------------------------|
| 0037   | `internal/filter/hcm/codec.go` head comment; PROGRESS Task 3                  |
| 0038   | `internal/filter/hcm/route.go` head comment lines 14/27–31; PROGRESS Task 4   |
| 0039   | `internal/filter/hcm/actions.go` head comment line 36; PROGRESS Task 5        |
| 0040   | `internal/filter/hcm/config.go` line 41 (parseFilter docstring); Task 7       |
| 0041   | `internal/filter/hcm/config.go` lines 31, 41; Task 7; SPEC §9                 |
| 0042   | `internal/filter/hcm/config.go` line 107 error text; Task 7                   |
| 0043   | `test/differential/fixture/fixture.go` lines 82, 99; Task 13                  |
| 0044   | `docs/envoy-go/BEHAVIOR_CONTRACT.md` line 222; Task 17; fixture-0003 driver   |

ADR-0043 ends with a slightly malformed final line (`Lands in Task 13 (first use site of the orchestration branch). **Supersedes (informal):** the implicit byte-comparison-only contract.` appears twice — once in the body and once after ADR-0044). See M-1 below — cosmetic.

### (3) SPEC §3 gates are evidence-backed — PASS (re-verified)

I re-ran every in-scope gate from this impl worktree on a fresh shell. Quoting verbatim:

```
$ go build ./...
(empty — exit 0)

$ go vet ./...
(empty — exit 0)

$ golangci-lint run ./...
(empty — exit 0)

$ go test -count=1 -timeout=5m ./internal/... ./test/helpers ./test/fixtures/... ./cmd/...
ok  	github.com/esalaine/envoy-go/internal/admin       0.040s
ok  	github.com/esalaine/envoy-go/internal/bootstrap   0.007s
ok  	github.com/esalaine/envoy-go/internal/cluster     0.006s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm  0.007s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy 0.006s
ok  	github.com/esalaine/envoy-go/internal/listener    0.008s
ok  	github.com/esalaine/envoy-go/internal/tls         0.017s
ok  	github.com/esalaine/envoy-go/test/helpers         0.004s
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver  0.002s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver       0.002s
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver 0.002s
ok  	github.com/esalaine/envoy-go/cmd/envoy-go         1.022s
```

Differential suite (`./test/differential/`) — re-ran from a fresh shell with the reference Envoy container live:

```
$ go test -count=1 -timeout=12m ./test/differential -run 'TestDifferential' -v
...
--- PASS: TestDifferential (5.51s)
    --- PASS: TestDifferential/0000-tcp-echo (1.61s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.26s)
    --- PASS: TestDifferential/0002-tls-tcp (1.38s)
    --- PASS: TestDifferential/0003-http11-routing (1.26s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential  5.586s
```

Gate (a) green: `0003-http11-routing` PASS. Gate (b) green: `0000-tcp-echo` / `0001-tcp-proxy-rr` / `0002-tls-tcp` all PASS without regression. Gate (c) — conformance — confirmed N/A by SPEC §3 row (c) ("h2spec is phase 05; no project-internal h1spec; vacuously green"); confirmed in SPEC text on `SPEC.md:69`.

Gate (d) — fuzz targets at 30s — re-ran every target on a fresh shell:

```
$ go test ./internal/filter/hcm -run=FuzzHCMConfigParse -fuzz=FuzzHCMConfigParse -fuzztime=30s
fuzz: elapsed: 30s, execs: 3975393 (147235/sec), new interesting: 21 (total: 472)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/hcm  31.054s

$ go test ./internal/bootstrap -run=FuzzBootstrapLoad -fuzz=FuzzBootstrapLoad -fuzztime=30s
fuzz: elapsed: 30s, execs: 482458 (0/sec), new interesting: 22 (total: 945)
PASS
ok  	github.com/esalaine/envoy-go/internal/bootstrap   31.082s

$ go test ./internal/filter/tcpproxy -run=FuzzTcpProxyFilter -fuzz=FuzzTcpProxyFilter -fuzztime=30s
fuzz: elapsed: 30s, execs: 4071288 (150252/sec), new interesting: 7 (total: 515)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy 31.056s

$ go test ./internal/tls -run=FuzzTLSContextParse -fuzz=FuzzTLSContextParse -fuzztime=30s
fuzz: elapsed: 30s, execs: 5259189 (606057/sec), new interesting: 22 (total: 525)
PASS
ok  	github.com/esalaine/envoy-go/internal/tls         31.046s

$ git status --porcelain
(empty — no testdata/fuzz/ pollution; ADR-0018 discipline upheld)
```

Gate (e) green. Gate (f) — REVIEW.md — produced by this document.

The PROGRESS verification block's quoted outputs are consistent with my live re-run on the same package set. Counts of `new interesting` per fuzz run differ (independent runs build their own coverage trace) but the PASS verdict and exec counts are within natural-variance band of the implementer's transcript. The block is not a synthetic transcript.

### (4) BEHAVIOR_CONTRACT.md HTTP/1.1 subsection is consistent with implementation — PASS

I cross-read the HTTP/1.1 subsection (lines 220–258) against the implementation:

- **"Decoded response body bytes for `direct_response` 2xx paths"** (asserted) — fixture-0003 driver concatenates the 9 `/health` 200 `OK\n` bodies into the `Drive*` byte stream; `ExpectBodyEquivalent: true` on the per-request HTTPExpectations leg. Matches.
- **"Route-match selection"** (asserted, witnessed via distribution) — `AssertDistribution` checks `[3,3,3]` per side over the 9 router-action requests. Matches.
- **"Decoded response body bytes for routed-to-upstream requests"** (NOT asserted) — fixture-0003 driver `drive()` deliberately does NOT concatenate `/api/v1/<n>` bodies; the file-level rationale comment names "ref and subj RR may start at different endpoints (STRICT_DNS vs STATIC initial pick)". Matches.
- **"Local-reply body bytes for 4xx/5xx"** (NOT asserted) — `/missing/<n>` bodies also not concatenated; comment names the divergence ("Envoy: HTML/JSON; envoy-go: 'not found\n'"). Matches.
- **Header allow-list extensions** (`Server`, `Content-Length`, `Transfer-Encoding`, `x-envoy-*`, `x-forwarded-*`, `x-request-id`) — verified row-by-row in `BEHAVIOR_CONTRACT.md` table at lines 33–39 and present verbatim in `test/helpers/http_diff.go` `PhaseFourHTTPAllowList`. Matches.
- **Framing-divergence-permitted** — `Content-Length` and `Transfer-Encoding` allow-list entries are in the runner's allow-list; the runner reads responses via `http.ReadResponse` (decodes both transparently). Matches.
- **Upstream connection re-use NOT asserted** — `routerAction.do` opens a fresh dial per request via `Cluster.Dial(ctx)` and `defer upstream.Close()`. Matches ADR-0039 verbatim.

The subsection is well-written and accurate.

### (5) ADR honour-check across ADR-0037..0044 — PASS (with one I-class clarification)

| ADR    | What I checked | Status |
|--------|---------------|--------|
| 0037 (stdlib `net/http` codec) | `internal/filter/hcm/` does NOT import `net/http/httputil`; does NOT import `net/http.Server`; does NOT define an `http.Handler`. `net/http.ReadRequest`, `Request.Write`, `http.ReadResponse`, `Response.Write`, and `http.StatusText` are the only stdlib surfaces consumed. The `cmd/envoy-go/main_test.go` HCM-smoke test reaches the binary via raw `net.Dial` + handcrafted request bytes (correct discipline). | PASS |
| 0038 (route match: prefix bytewise + path exact) | `route.go` defines exactly `matchPath` and `matchPrefix`; `config.go` `buildMatch` rejects `safe_regex`, `path_separated_prefix`, `connect_matcher`, `case_sensitive=false`, `headers[]`, `query_parameters[]`, `runtime_fraction`, `dynamic_metadata[]`, `tls_context`. Test `TestMatchPrefix` explicitly asserts `/apifoo` matches `/api` (the documented bytewise divergence from segment-aware Envoy semantics). | PASS |
| 0039 (per-request fresh upstream dial) | `routerAction.do` calls `a.cluster.Dial(ctx)` on every invocation and `defer upstream.Close()` on the next line. No pool struct, no idle-list, no `sync.Pool`. The fixture-0003 distribution `[3,3,3]` is the witness that per-request dials drive the RR pick. | PASS |
| 0040 (HTTP-filter framework subset = router-only, no iteration protocol) | `requireRouterOnlyHTTPFilters` validates `http_filters` length is exactly 1, name is `envoy.filters.http.router`, type_url is `Router`. The Router proto body unmarshals successfully but no Router field is read (the test `TestParseFilter_HappyPath` confirms the path passes with an empty Router). The connection loop calls `entry.action.do` directly — no `decode_headers` / `Continue` / `StopIteration`. | PASS |
| 0041 (HCM `stat_prefix` REQUIRED + silently-ignored set) | `parseFilter` errors on missing `stat_prefix`; on `codec_type=HTTP2/HTTP3`; on RDS / ScopedRoutes route_specifier; on non-1 vhost count; on non-`["*"]` domains. Every other top-level HCM proto field documented in ADR-0041 as silently-ignored is in fact silently ignored (parseFilter does not read them). The `Filter` struct stores `statPrefix string` (matches SPEC §10 #9 settled). | PASS |
| 0042 (HTTP-filter chain shape exactly `[router]`) | Empty chain rejected (`http_filters: got 0 entries, want exactly 1`); two-entry chain rejected; non-router name rejected; wrong type_url rejected. Test coverage in `config_test.go:186-209`. | PASS |
| 0043 (`HTTPExpectations` interface) | Interface declared verbatim per ADR text in `test/differential/fixture/fixture.go:100-102`. The `HTTPRequestExpectation` struct matches the ADR's `{Method, Path, ExpectStatus, ExpectBodyEquivalent}` shape. The runner orchestration branch (lines 166–192) implements the per-request status / body / header diff with the type-assertion gate (drivers that don't implement the interface are unaffected). | PASS |
| 0044 (BEHAVIOR_CONTRACT HTTP/1.1 subsection + driver enforcement) | Subsection lives at `BEHAVIOR_CONTRACT.md:220-258`. Header allow-list table extended with the six new rows (`Server`, `Content-Length`, `Transfer-Encoding`, `x-envoy-*`, `x-forwarded-*`, `x-request-id`). Fixture-0003 driver enforces the contract (per-cluster `[3,3,3]`; `/health` body byte-equal — `ExpectBodyEquivalent: true`; `/api/v1/N` and `/missing/N` status-only — `ExpectBodyEquivalent: false`); driver file-level rationale comment names the bodies-not-concatenated decision. | PASS |

### (6) No `testdata/fuzz/` seed-corpus pollution — PASS

```
$ git status --porcelain
(empty — exit 0)
```

Four 30-second fuzz runs on this re-verification did not promote any seed entries. Per ADR-0018's discipline, this is the expected outcome of a clean run.

---

## Findings

### Critical

None. The phase is functionally correct, the gates are green, ADRs are honoured, and the BEHAVIOR_CONTRACT update is consistent.

### Important

**I-1. `errCloseAfterAction` sentinel is wired up but never returned — upstream `Connection: close` is not honoured downstream.** `internal/filter/hcm/actions.go:20` declares `errCloseAfterAction`. `internal/filter/hcm/connection.go:67` checks `errors.Is(actErr, errCloseAfterAction)`. SPEC §5.3 explicitly requires "also break if the action's response carried `Connection: close`". The router action currently never inspects `resp.Header.Get("Connection")` and never returns the sentinel; the direct_response action correctly cannot trigger it (it's the docstring's stated invariant). The result: when the upstream backend sends `Connection: close` in its response, the HCM does NOT close the downstream after delivering that response — it loops to the next `http.ReadRequest`. Phase-04 fixture-0003 backends DO send `Connection: close` (fixture HTTP-echo backend at `runner_test.go:356-357` writes `Connection: close`), but the test client (`helpers.HTTPRoundTrip`) sets `Connection: close` on every request, which separately drives `closeAfterRequest=true`. So the missing wiring is masked by the request-side close.

In the phase-04 differential gate this divergence is invisible. The risk is the masking: a future phase-05+ HTTP/2 fixture or any consumer that pipelines requests on a keep-alive downstream while the upstream sends `Connection: close` will see one extra round-trip's worth of stale-conn bytes before the next read errors out. There is also no test exercising the close-after-action path — `actions_test.go:163` checks `errors.Is(err, errCloseAfterAction)` but only inside a "this should not error" branch, not as a positive assertion. SPEC §10 #3 was settled to "sentinel error mechanism (option a)" but that mechanism never landed.

**Fix:** In `routerAction.do`, after `http.ReadResponse` succeeds, check `strings.EqualFold(resp.Header.Get("Connection"), "close")` (and/or `resp.Close == true`). If true, return `errCloseAfterAction` after `resp.Write(bw)` succeeds. Add a `connection_test.go` test that exercises the path: make `loopbackHTTPEcho`-equivalent send `Connection: close` and assert the connection loop closes after one request even though the request had no `Connection: close` header. ~10 LOC + 1 test.

**I-2. Router action signals `503` on a context-cancellation by leaving the test ambiguous.** `actions_test.go:173-192` (`TestRouterAction_DoCtxCancel`) cancels `ctx` then calls `a.do(ctx, req, bw)`. The test asserts the response begins with `HTTP/1.1 503 ` — which is correct (Cluster.Dial returns ctx.Err() → routerAction maps to 503 local reply) — but the test silently swallows the return value of `a.do` (`_ = a.do(ctx, req, bw)`). If the implementation later starts returning ctx.Err() up rather than synthesizing a 503, this test will silently keep passing. SPEC §7's failure-mode table classifies "Cluster.Dial failure" runtime → "writeStatusReply 503; do not close" — the action is supposed to swallow the error and the test should assert `err == nil`. Tightening the test would catch any regression where the action propagates ctx-cancellation up instead of mapping it to 503.

**Fix:** Change `_ = a.do(ctx, req, bw)` to `if err := a.do(ctx, req, bw); err != nil { t.Errorf("ctx-cancel should map to 503 local reply, not propagate err: %v", err) }`. One-line tightening.

**I-3. `routerAction.do` does not propagate ctx into upstream `req.Write` or `http.ReadResponse`.** `req.Write(upstream)` and `http.ReadResponse(bufio.NewReader(upstream), req)` are blocking calls with no ctx awareness. If the downstream's ctx is canceled mid-write or mid-read, the action will keep blocking on the upstream socket until the upstream closes or its OS-level timeout fires. The `Cluster.Dial(ctx)` part respects ctx, but the post-dial path does not. SPEC §11 (Risks and mitigations) lists the dial respect; it does NOT list the post-dial. ADR-0039 documents that pooling and timeouts are deferred to the upstream-robustness family — which arguably covers this — but a single `upstream.SetDeadline(ctx_deadline)` call before `req.Write` would close the gap without introducing pooling. Phase-04 fixture-0003 drives every round-trip on a finite ctx (`90s` per-fixture timeout in the runner), so the divergence is bounded and not exercised maliciously, but a fixture with a shorter upstream-stall would deadlock until the runner's outer 90s.

**Fix (one-liner, optional):** before `req.Write(upstream)`, if `dl, ok := ctx.Deadline(); ok { _ = upstream.SetDeadline(dl) }`. This propagates the ctx deadline to the upstream socket. Could ship in the phase-05 first commit alongside I-1.

**I-4. Phase-03 REVIEW Minor 5 (`internal/cluster/manager.go` "phase 02" stale error texts) carries forward unchanged.** SPEC §12 noted this would be RESOLVED-OPPORTUNISTIC if phase 04 touched cluster/manager.go. It did not. Lines 83/86/89/140 of `internal/cluster/manager.go` still say "phase 02 supports only STATIC" / "phase 02 supports only ROUND_ROBIN". An operator running envoy-go from phase 04 sees error messages naming phase 02 — confusing. The string content is correct on the merits (cluster_discovery_type / lb_policy support has not expanded since phase 02), but the version cite is stale.

**Fix:** Change "phase 02" to "phase 04" (or, better, drop the phase-N qualifier — the constraint is project-wide until a future phase relaxes it). 4-line edit. Could ship in the phase-05 first commit.

### Minor

**M-1. ADR-0043 ends with a duplicated tail line.** `DECISIONS.md:1448` reads `Lands in Task 13 (first use site of the orchestration branch). **Supersedes (informal):** the implicit byte-comparison-only contract.` — but ADR-0044 begins on line 1403 (so 1448 falls inside ADR-0043 territory, not at its end). The same-shape line for ADR-0044 lands at line 1446. Two sequential "Lands in Task ..." lines in ADR-0043's body, one of which appears to be a paste-leftover from ADR-0044's drafting. Cosmetic; harmless to readers but worth a tidy.

**Fix:** Delete the trailing line at `DECISIONS.md:1448` so ADR-0043's body ends cleanly at its real Consequences block. One-line delete.

**M-2. ADR-0043's "Doctrine" header reads `D-3.4, D-3.5` but the supersession is informal.** ADR-0034 (which ADR-0043 evolves from) has the same `Supersedes (informal)` qualifier. Mechanical inconsistency between ADR-0034 (informal supersession on a separate line) and ADR-0043 (informal supersession appended on the body's tail line). Future ADR-cleanup pass could regularise.

**M-3. `connection.go:60` `closeAfterAction := false` is presently unreachable-true (dead branch).** Because no action ever returns the sentinel (per I-1), the `closeAfterAction` local variable is always `false` at the close check on line 84. Until I-1 is fixed, the variable could be deleted and the check reduced to `if closeAfterRequest { return }`. After I-1 is fixed the variable is needed. Suggestion: keep the variable as-is (it documents the contract) and flag I-1 as the real fix.

**M-4. `internal/listener/manager.go` filter-handler-shape concern from phase 03 still surfaces.** Phase 03 REVIEW M-2 noted `Stop()` writes `rt.netLn = nil` while `Listeners()` reads `rt.netLn` without locking. Phase 04 does not touch this surface. The race is visible to `go test -race ./internal/listener` if any future test holds a reference and races; the implementer ran `go test -race ./...` per PROGRESS Task 17 and got clean, which is good evidence the current test surface does not trigger it. Carries forward.

**M-5. `routerAction.do` doc-comment claims a 502 is returned on `req.Write` failure, then "non-error return is a sentinel" — actually it returns nil after writing 502.** `actions.go:39-43` lists the failure-class mapping ("Request.Write error → 502 local reply, do returns nil"). The code (lines 58–59) matches the doc. Good. But the SPEC §7 failure-mode table on `SPEC.md:393-395` says `routerAction.do: req.Write to upstream fails → "writeStatusReply(bw, 502, ""), flush; close upstream; do not close downstream"` — the "close upstream" requirement is satisfied (the `defer upstream.Close()` fires when `do` returns), and "do not close downstream" is satisfied (do returns nil, the connection loop continues). The discrepancy between SPEC §7's three-step description and the action's two-step implementation (no explicit upstream close, just defer) is a doc-vs-implementation cosmetic. The defer is the correct mechanism.

**M-6. Fixture-0003 driver's `referenceTmpl`/`subjectTmpl` are large heredoc YAML strings — same pattern as fixtures 0001/0002, no new concern but worth carrying forward to the phase-08 / observability phase that introduces a structured `expectations.yaml` per ADR-0019. The prose-heavy `expectations.yaml` for fixture-0003 (51 lines) extends the phase-02 / phase-03 prose convention; ADR-0019's structured-form deferral still applies.**

**M-7. `Filter.statPrefix` is stored but never consumed in phase 04.** `filter.go` / `config.go` populate `statPrefix` from the proto; no in-tree consumer reads it. ADR-0041 says the field is forward-look for phase-06 stats, which is correct, but the unused-field flag is invisible to `golangci-lint` (the field is exported-shaped but private). When phase 06 lands, that consumer should hook into this field; until then, a comment like `// CONSUMED-BY-PHASE-06` or a small "ensure non-empty post-parse" assertion in `parseFilter` (which already exists at line 60) would surface the contract. Minor.

---

## Strengths

- **Clean ADR discipline.** Each ADR is single-concern (0037 = wire codec; 0038 = route match; 0039 = dial strategy; 0040 = filter framework; 0041 = HCM ignored-set; 0042 = HTTP-filter-chain shape; 0043 = HTTPExpectations driver extension; 0044 = BEHAVIOR_CONTRACT subsection). No phase-04 ADR bundles unrelated concerns. Numbering is sequential without gaps.
- **`internal/filter/hcm/` package is small and well-factored.** Twelve files (`doc`, `filter`, `config`, `route`, `actions`, `codec`, `connection`, `fuzz_test` + five `*_test`); average file size 80 lines; clear separation of concerns; no file does more than one thing. The `routeMatch` interface + `matchPath`/`matchPrefix` impls (planner picked option (a) of SPEC §10 #1) is the right Go idiom — extends cleanly to the phase-07 expanded predicate set.
- **Doctrine compliance (D-3.2) is held without sleight-of-hand.** No `net/http/httputil`, no `net/http.Server`, no `http.Handler`. The connection loop drives `http.ReadRequest` / `Request.Write` / `http.ReadResponse` / `Response.Write` directly — those are the four parser/serializer primitives only.
- **TDD discipline is visible in PROGRESS.** Task 4 records "wrote failing route-match tests first; saw them fail; then wrote `route.go`"; Task 6 likewise on connection.go ("`http.ReadRequest` returns ParseError on garbage — confirmed before implementing 400-on-error path"). Red-state captures are how you prove tests don't trivially pass.
- **PROGRESS verification block is rigorous.** Commit `7649a19` quotes every gate verbatim (build, vet, lint, test, four 30s fuzz runs, post-run `git status`, full differential suite output with container lifecycle traces). Reviewer can cross-check everything without re-running.
- **Phase-03 REVIEW carryover triage is honest.** SPEC §12 names every phase-03 Minor by number, rules each one in/out, and gives rationale per item. The two RESOLVED-OPPORTUNISTIC entries (M-4 / M-5) honestly fall back to "carries forward" when phase 04 doesn't touch the relevant file — no fabricated resolution. The phase-04 PLAN.md preamble's STATE.md fabrication note (D-3.4 finding from brainstorming) is also surfaced honestly in §12's tail.
- **`routeMatch` interface vs tagged-union.** The planner picked option (a) per SPEC §10 #1. The `routeMatch.matches(path string) bool` interface is the project-correct shape — phase 07's expanded predicate set (regex, segment-aware prefix, header-aware match) extends it without breaking changes.
- **Per-cluster RR distribution `[3,3,3]` is the right witness for route-match equivalence.** The fixture driver's choice to NOT byte-compare per-request `/api/v1/N` body bytes — and to rely on `[3,3,3]` distribution + status-200-per-request as the equivalence witness — is correct under ADR-0027's STATIC-vs-STRICT_DNS regime. The driver's file-level rationale comment names the constraint precisely.
- **HTTP-echo backend isolation choice (handcrafted bufio per SPEC §10 #6 picked option (b)) is consistent with the rest of the test/helpers/ surface.** The runner's `acceptHTTPEchoCounting` reads one request via stdlib `http.ReadRequest`, drains the body, writes a hand-rolled response — same shape as the TCP-echo `acceptEchoCounting` and same shape as fixture-0002's PKI generator's pattern of stdlib-where-possible-handcrafted-where-needed.
- **HTTPExpectations type-assertion gate is the right backward-compatibility shape.** The runner's `if he, ok := d.(fixture.HTTPExpectations); ok` allows phase-02/03 drivers (0000, 0001, 0002) to remain unchanged. ADR-0043 documents the additive-evolution intent and sets up phase-05 to reuse the same shape.
- **Fuzz seed corpus matches SPEC §4.1 exactly.** Three seeds: well-formed HCM Any (one direct_response + one router route + router-only http_filters list); well-formed HCM type_url with empty value; non-HCM type_url. Fuzz body asserts both no-panic and `hcm:` error prefix per SPEC. 30s budget per ADR-0018.
- **Subject-side `Server: envoy` (vs `envoy-go`) reaffirmation per SPEC §10 #12.** The planner picked the right answer — keeping `Server: envoy` lets the BEHAVIOR_CONTRACT header allow-list use the simpler "presence-only" rule (no per-value handling for the `Server` row). ADR-0014 stays as the single source of truth.

---

## Recommendation

**APPROVED WITH FOLLOW-UPS.** The phase is correct, well-tested, and well-documented. Zero Critical findings. Four Important findings, none of which is a blocker:

1. **I-1**: Wire `errCloseAfterAction` into `routerAction.do` (check upstream's `Connection: close`). Add a `connection_test.go` test that asserts the close path. ~10 LOC + 1 test.
2. **I-2**: Tighten `TestRouterAction_DoCtxCancel` to assert the action returns nil (not just "produces 503 in the buffer"). One-line tightening.
3. **I-3**: Optionally propagate ctx deadline to upstream `SetDeadline` before `req.Write` to avoid post-dial hang. One-liner; could be deferred to phase-05.
4. **I-4**: Refresh `internal/cluster/manager.go` "phase 02" error strings to "phase 04" (or drop the phase qualifier). 4-line edit. (Phase-03 REVIEW M-5 carry-forward — RESOLVED-OPPORTUNISTIC slot still open.)

Seven Minor findings (M-1 through M-7) carry forward to future-phase REVIEW triage. **M-1** (ADR-0043 trailing duplicate line) is the cleanest one-line cleanup; could ship with the I-1..I-4 follow-up commit.

The single most important context to surface to the phase-05 planner: **fixture 0003 still does not differentially exercise upstream TLS** (per phase-03 ADR-0035 — phase 04 explicitly didn't try). Phase 05 (HTTP/2 — almost always over TLS) is the natural place to close this loop, either by extending `test/differential/harness.go` to spawn TLS-capable HTTP/2 backends or by introducing a naturally-HTTPS HTTP/2 fixture whose upstream dial validates `Cluster.Dial`'s TLS branch cross-proxy.

Phase 04 is ready to advance to lifecycle-state 6.

**Verdict: APPROVE**
