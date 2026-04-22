# Phase 00 — Bootstrap Review

**Reviewer:** superpowers:code-reviewer (dispatched 2026-04-22)
**Range:** d8c5404..b8571c7
**Verdict:** APPROVED-WITH-FIXES

## Summary

Phase 00 delivers what it set out to deliver: a real (not stubbed) differential test
harness that starts the pinned upstream Envoy image via testcontainers and compares
its TCP echo output byte-for-byte against an `envoy-go` subprocess. All 16 PLAN
tasks are traceable to atomic commits, all SPEC §12 checklist items land, and the
phase-done gates (SPEC §3 / §7.5) are satisfied with verbatim command outputs
quoted in `PROGRESS.md`. `go build ./...` and `go vet ./...` re-run cleanly at the
reviewed head. Findings are all Important-or-lower: two documentation drift items
(ADR-0010's promised `BEHAVIOR_CONTRACT.md` note never landed; docstring claims
Cmd-override indirection when the runner actually does string replacement), one
minor code-hygiene issue (dead-code branch in `CompareBytes`), and two small
robustness notes for future phases. None of them require Go-code changes that
would invalidate the verification, so the phase is safe to advance to
lifecycle-state 6 after fixing the documentation drift in-place.

## Strengths

- **Differential harness is real.** `test/differential/harness.go:96-141` launches
  the pinned Envoy image by `SHA256` (not tag) via `testcontainers-go`, and
  `test/differential/harness.go:165-213` really builds `cmd/envoy-go` and execs it
  as a subprocess, waiting on the `envoy-go ready on` stdout sentinel. No mocks.
- **Envoy image pinned by SHA256 end-to-end.** `docs/envoy-go/ENVOY_TARGET.md:4`
  carries the SHA256, and `test/differential/harness.go:102` passes `pin.SHA256` —
  not `pin.Tag` — as the image reference. Doctrine D-3.7 satisfied mechanically,
  not just documentarily.
- **Container-leak paths closed.** `test/differential/harness.go:119,124,131`
  each call `_ = c.Terminate(ctx)` before returning an error, preventing
  orphan Envoy containers on partial startup (Task 10 self-review catch; commit
  `33c5a2a`).
- **ADR discipline is visible.** Every non-trivial deviation from PLAN earned an
  ADR in-session: ADR-0009 for the golangci-lint pin bump, ADR-0010 for the
  `V4_ONLY` DNS workaround, ADR-0011 for the `docker/docker` pin. Each ADR
  explains the problem, the options considered, and the chosen fix.
- **TDD evidence is in PROGRESS.md.** Every task shows a RED failing-compile
  run followed by a GREEN run with the test body actually executing (e.g.
  `PROGRESS.md:107-131` for the config parser, `:195-212` for `CompareBytes`).
  This is not theater — the RED outputs show `undefined: <sym>` errors that
  only disappear when the implementation lands.
- **Splice workaround is correctly identified and documented.**
  `cmd/envoy-go/main.go:59-64` wraps `net.Conn` to block Go's Linux `splice(2)`
  optimization that silently drops bytes when source and destination are both
  TCP sockets under an echo backend. The comment explains *why*, not just *what*.
- **Fixture discovery tolerates an empty `test/fixtures/` directory**
  (`test/differential/runner_test.go:94-99`) so the harness works during
  intermediate states (e.g. between Task 12 landing and Task 13 landing). Small
  touch but saves a landmine.
- **CI workflow mirrors SPEC §3 precisely** — `lint-vet-test` runs vet, lint,
  and `-short` tests; `differential` runs the full differential suite and
  depends on `lint-vet-test` so it cannot hide a lint regression.

## Findings

### Critical (must fix before lifecycle-state 6)

*None.*

### Important (should fix, reviewer's judgment call)

1. **ADR-0010 promises a `BEHAVIOR_CONTRACT.md` note that never landed.**
   `docs/envoy-go/DECISIONS.md:269-271` says: *"All fixtures whose reference
   bootstrap uses `host.docker.internal` in a `STRICT_DNS` cluster set
   `dns_lookup_family: V4_ONLY` on that cluster. This is codified by… A note in
   `docs/envoy-go/BEHAVIOR_CONTRACT.md` (see Consequences) making this the
   standard for future TCP/HTTP/1.1/HTTP/2 fixtures using
   `host.docker.internal`."* A `grep -n "V4_ONLY\|host.docker.internal\|dns_lookup_family"
   docs/envoy-go/BEHAVIOR_CONTRACT.md` returns nothing — the file was never
   updated. This is a doctrine-D-3.4 context-isolation failure: a future phase's
   author will read the ADR, expect the contract rule, and not find it. It is
   also a doctrine-D-3.5 internal-consistency failure of the ADR itself. Fix:
   either append the V4_ONLY rule under a new `## Transport and DNS` (or
   `## Test harness host networking`) subsection in `BEHAVIOR_CONTRACT.md`, or
   supersede ADR-0010 with an ADR-0012 that drops the contract-update clause.

2. **`envoy.yaml` docstring misrepresents the substitution mechanism.**
   `test/fixtures/0000-tcp-echo/envoy.yaml:11` says *"The driver substitutes
   the backend port in via the runner's per-fixture template hook (see
   runFixture / SubjectConfig in test/differential)"* and line 46 says
   *"placeholder; runner regenerates with backend port via Cmd override"*. The
   actual mechanism is neither a template hook nor a Cmd override — it is a
   literal `strings.Replace` of `"port_value: 0"` in the bootstrap YAML before
   the container starts (`test/differential/runner_test.go:61`). The comment in
   `driver/driver.go:102-105` gets it right. Fix: align the two
   `envoy.yaml` comments with the actual mechanism, noting that phase 01 will
   replace the `strings.Replace` with real templating.

3. **`CompareBytes` has an unreachable `if eq` branch.**
   `test/differential/diff.go:20-34`: `eq` is set `true` at line 21, never
   modified (the body of the loop returns early on mismatch), then re-checked
   at line 31. This is dead code that survives lint only because `staticcheck`
   does not flag "always-true constant guard after an early-returning loop" as
   an issue. The function's behavior is correct, but the control flow is
   confusing — a reader would reasonably ask "when is `eq` false if it's set
   true and never written?" Fix: delete the `eq` variable and replace the
   `if eq { ... }` with the direct `return Verdict{Equal: true}, nil`. One
   line of churn; no behavior change. (Because it's pure cosmetic, it's
   Important rather than Critical — but it's the one `diff.go` reader-cost
   artifact in the phase.)

4. **Static `refContainerListenerPort = 15000` forces fixture collisions.**
   `test/fixtures/0000-tcp-echo/driver/driver.go:15` hardcodes `15000`, and the
   bootstrap YAML string at `driver/driver.go:78` embeds the same literal. If
   two fixtures are ever run in parallel (or if a later fixture also picks
   `15000`), the in-container port is a global singleton for the test process
   and collides. Fix (cheap): plumb the port as a constructor parameter when
   the fixture is registered, or generate it from a counter. (Not Critical
   because the phase-00 harness is serial and the runner `t.Run`s fixtures in
   order; but a future phase that enables `t.Parallel()` or adds a second TCP
   fixture will hit this.)

### Minor (nice-to-have, can defer)

1. **`scanForLine` leaks a goroutine on context timeout.**
   `test/differential/harness.go:58-83`: when `ctx.Done()` fires first, the
   inner goroutine keeps `br.ReadString('\n')`-ing on a pipe the caller may
   never drain. In practice the caller's next step is `cmd.Process.Kill()`,
   which closes the pipe and unblocks the goroutine, so the leak is bounded by
   the caller's cleanup discipline. Worth noting for when the harness grows
   more long-lived helpers. Fix: wrap the goroutine's read in a loop that
   checks a `ctx.Done()` channel via a second select, or close the underlying
   pipe in a `defer` the caller controls.

2. **`StartSubjectProxy` rebuilds the binary per test call.**
   `test/differential/harness.go:171-176` runs `go build -o <tmp>/envoy-go
   ./cmd/envoy-go` every time. With Go's build cache this is ~100ms on a warm
   machine, but `TestReferenceProxy_Starts`, `TestSubjectProxy_StartsAndReports`,
   and `TestDifferential` each incur it independently, and the differential
   suite scales linearly with fixture count. For phase 00 (one fixture) it's
   harmless; for phase 05+ consider a package-level `sync.Once` build or a
   `testing.TestMain` pre-build. Not a phase-00 fix.

3. **`CompareBytes`'s signature returns `error` that it never populates.**
   `test/differential/diff.go:19`: `func CompareBytes(ref, subj []byte)
   (Verdict, error)` — the function cannot fail. Either drop the error (pure)
   or reserve it for a future richer comparator that can (e.g. when
   `expectations.yaml` tolerances land). The current callers dutifully check
   the always-nil error. Not fixed in phase 00 because changing the signature
   would churn three callers, but worth a TODO for phase 02 when
   `BEHAVIOR_CONTRACT.md` gains its first real tolerance rule.

4. **CI does not cache golangci-lint binary; re-installs on every run.**
   `.github/workflows/ci.yml:21-23` uses `golangci/golangci-lint-action@v6`,
   which downloads the pinned v1.64.8 binary each run. GitHub's action cache
   makes this fast but not free. Not a correctness issue; a performance note
   for when CI time becomes a gate.

5. **ADR numbering is out of physical order in `DECISIONS.md`.**
   The file is `0001, 0002, 0003, 0004, 0005, 0006, 0008, 0007, 0009, 0010,
   0011`. Per `BOOTSTRAP_PROMPT.md §4.1` invariant 4 ("ADR-numbered,
   append-only"), the *numbers* are authoritative, not the physical order, so
   this is compliant. But a reader using the file as an append-only log will
   find ADR-0008 before ADR-0007, which is confusing. Not worth rewriting; log
   a note that future ADRs keep strict chronological-numerical order.

6. **`main_test.go` `waitForReady` uses a busy loop with no back-off when the
   stdout reader returns an empty line.** `cmd/envoy-go/main_test.go:113-127`
   calls `br.ReadString('\n')` which blocks until a newline, so there's no
   spinning in practice — but if the subject ever emitted a partial line and
   then stalled, the loop would wait on `ReadString` forever up to `timeout`.
   Current behavior is correct for the current subject (one full line, then
   stdin-closed). Fix is not required.

## Axis-by-axis assessment

### Axis 1: PLAN.md fidelity

Walked all 16 tasks against the diff and PROGRESS.md. Every task produced
the expected code/test and an atomic commit with a matching PROGRESS.md entry.
Deviations from PLAN text are all ADR'd.

| Task | PLAN intent | Actual commit(s) | PROGRESS | ADR for deviation | Verdict |
|---|---|---|---|---|---|
| 1 | `go mod init` | f31501f | yes | — | pass |
| 2 | ADR-0005, ADR-0006 | 31172f1 | yes | — | pass |
| 3 | 12 `internal/*/doc.go` | f2e4576 | yes | — | pass |
| 4 | Envoy pin + ADR-0008 | 0819740 | yes (with pull transcript) | — | pass |
| 5 | `.golangci.yml` | 1a57bd3 | yes | — | pass |
| 6 | Config schema + parser (TDD) | 9756b78 + e335ce7 | yes with RED+GREEN | ADR-0007 | pass |
| 7 | TCP-pump main + helpers | d733e08 | yes with splice deviation note | inline-in-PROGRESS (D-3.5 spirit; splice is a Linux-only code-level workaround, not a cross-phase decision) | pass |
| 8 | Diff + hex-dump | c5bb5c2 | yes | — | pass |
| 9 | Pin loader + ready scanner | 9b4e86f + 81ae9ee | yes (DONE_WITH_CONCERNS → follow-up) | — | pass |
| 10 | Reference proxy | a789b18 + 33c5a2a | yes (SHA256+leak fix follow-up) | — | pass |
| 11 | Subject proxy | 90d1c30 | yes | — | pass |
| 12 | Runner | d16bd35 | yes (PLAN-import-cycle deviation disclosed) | ADR-0010 / ADR-0011 in task 13 round | pass |
| 13 | Echo fixture | 5e96def + 59978de + a1714cb + 9a41b9e | yes (4 deviations → 3 new ADRs) | ADR-0009, ADR-0010, ADR-0011 | pass |
| 14 | `test/conformance/doc.go` | a7e9e28 | yes | — | pass |
| 15 | CI workflow | 35024ca | yes | ADR-0009 (pin) | pass |
| 16 | Green CI equivalent | 496484b | yes (full transcripts) | — | pass |

No task was silently skipped. No scope-creep introduced.

### Axis 2: SPEC.md §12 acceptance checklist

| Item | Status | Evidence |
|---|---|---|
| All §4 paths exist with described contents | PASS | `ls internal/ test/ test/fixtures/ test/differential/` matches §4 tree. The only addition is `test/differential/fixture/` (leaf package to break a Go import cycle), disclosed and explained in Task 13 PROGRESS. |
| `ENVOY_TARGET.md` populated (tag + SHA256, not placeholder) | PASS | `docs/envoy-go/ENVOY_TARGET.md:3-4` contains tag `envoyproxy/envoy:v1.37.2` and SHA256 `c5e8…18bd`. Refresh procedure present. |
| `go vet`, `golangci-lint`, `go test ./...`, `go test ./test/differential/...` green locally and in CI | PASS | PROGRESS.md `Verification` section quotes all four at exit 0. Reviewer re-ran `go build ./... && go vet ./...` from head — clean. |
| Echo fixture's `expectations.yaml` enumerates each §7.2 dimension | PASS | `test/fixtures/0000-tcp-echo/expectations.yaml:7-34` — 9 dimensions, each with either `applicable: true` + rule, or `applicable: false` + one-line reason. |
| CI workflow on push + pull_request, both jobs | PASS | `.github/workflows/ci.yml:3-5` (`on: push`, `on: pull_request`); jobs `lint-vet-test` and `differential` with `differential` depending on `lint-vet-test`. |
| ADRs for every §10 deferred decision landed | PASS | Module path: ADR-0006 (§10 #1). Envoy pin: ADR-0008 (§10 #2). Go version: no ADR needed (stayed at 1.23 floor per SPEC §10 #3; the toolchain actually used is 1.26.2 but `go.mod` declares `go 1.23.0`, satisfying the floor). golangci-lint pin: ADR-0009 (supersedes §10 #4 baseline on the version question). CI runner: no ADR (ubuntu-latest works, per SPEC §10 #5). YAML schema: ADR-0007 (§10 #6). |
| `STATE.md` advanced and `ROADMAP.md` row 00 set to `done` | PARTIAL (expected, per lifecycle) | `STATE.md` is at lifecycle-state 5 (pending this very review). `ROADMAP.md:31` still shows row 00 as `in-progress`. Per SPEC §3 / state machine §5 step 6, the transition to `done` is the final commit *after* this REVIEW.md lands approved. Ticking this box is the next-session action. |
| `PROGRESS.md` contains full log + gate outputs quoted verbatim | PASS | Every task entry has `**Commits:**`, `**Notes:**`, and a `**Outputs:**` block with pasted stdout. The final `Verification (lifecycle-state 4 → 5)` section quotes all six phase-done gates verbatim. |
| `REVIEW.md` approved | IN FLIGHT | This document. |

### Axis 3: Doctrine D-3.1–D-3.7

| Doctrine | Status | Evidence |
|---|---|---|
| D-3.1 Superpowers-first process | PASS | Brainstorming produced SPEC.md; writing-plans produced PLAN.md; TDD evidence in PROGRESS task-by-task (RED → GREEN); verification-before-completion section at PROGRESS bottom; this REVIEW.md is the requesting-code-review output. Each unexpected state (splice bug, golangci-lint install failure, import cycle, docker/docker API break) was resolved by systematic-debugging + ADR, not improvisation. |
| D-3.2 Hybrid implementation stance | PASS | Permitted foundations used: `testcontainers-go`, `gopkg.in/yaml.v3`, stdlib. No `httputil.ReverseProxy`, no Traefik/Caddy/fasthttp, no cgo, no GPL code. The `docker/docker` types imported are test-only (harness, not runtime — confirmed by grepping: it appears only under `test/differential/`). |
| D-3.3 Differential correctness beats internal fidelity | PASS | The echo fixture *really* byte-compares upstream Envoy v1.37.2 vs the envoy-go subprocess. `CompareBytes` is a straightforward byte slice comparator; the driver sends 10 identical payloads to both and diffs the concatenated response. No mocks anywhere in the fixture path. The reviewer verified the harness calls `testcontainers.GenericContainer` with `pin.SHA256` (not a mock) at `test/differential/harness.go:110-113`. |
| D-3.4 Context isolation | PASS (with one caveat → Important finding 1) | SPEC, PLAN, PROGRESS, REVIEW, ADRs are all stranger-readable. The one leak is ADR-0010 referencing a `BEHAVIOR_CONTRACT.md` entry that was never written — addressed as Important finding 1. |
| D-3.5 Decisions are written, not remembered | PASS | Three in-session deviations (golangci-lint pin bump, DNS V4_ONLY, docker/docker pin) each earned an ADR. ADR append-only discipline maintained — no landed ADR was edited in place (verified by `git log --all --oneline -- docs/envoy-go/DECISIONS.md` showing only append commits). |
| D-3.6 Every phase is a green build | PASS | All six phase-done gates (SPEC §3) exit 0 at the verification head. Reviewer independently ran `go build ./...` and `go vet ./...` at `b8571c7`; both clean, exit 0. |
| D-3.7 Version pinning | PASS | Envoy: tag `envoyproxy/envoy:v1.37.2` + SHA256, pinned in ENVOY_TARGET.md, referenced by SHA in `StartReferenceProxy`. Go: `go 1.23.0` in `go.mod`. golangci-lint: `v1.64.8` in `.github/workflows/ci.yml:23` and ADR-0009. testcontainers-go: `v0.27.0` in `go.mod`. docker/docker: `v24.0.7+incompatible` in `go.mod` (test-only) + ADR-0011. Upgrade procedure for the Envoy pin is documented at `ENVOY_TARGET.md:10-21`. |

### Axis 4: Phase-done gate (SPEC §3 / BOOTSTRAP_PROMPT §7.5)

Independently re-verified the simpler gates from a clean head:

- `go build ./...` — exit 0, no output (reviewer sanity check).
- `go vet ./...` — exit 0, no output (reviewer sanity check).
- `.golangci.yml` matches SPEC §5.5 baseline linters verbatim (govet, errcheck,
  staticcheck, unused, ineffassign, gofmt, goimports, misspell, revive) at
  `.golangci.yml:12-20`; `disable-all: true` ensures the set is exactly the
  nine declared.
- CI workflow (`.github/workflows/ci.yml`) mirrors SPEC §3: `lint-vet-test`
  runs vet, lint, unit tests (`-short`); `differential` depends on it and runs
  `go test ./test/differential/... -timeout 5m -v`.
- Differential harness is real (not stubbed): `StartReferenceProxy` uses
  `testcontainers.GenericContainer` with `pin.SHA256` as the image reference
  (`test/differential/harness.go:102,110`). `StartSubjectProxy` runs
  `exec.CommandContext(ctx, bin, "-c", cfgPath)` against the freshly built
  `cmd/envoy-go` binary (`test/differential/harness.go:183`).

Gate-by-gate roll-up (same as PROGRESS.md, cross-checked):

| Gate | Result | Evidence |
|---|---|---|
| (a) new/changed differential fixtures green | PASS | `TestDifferential/0000-tcp-echo` PASS in PROGRESS verification section |
| (b) pre-existing fixtures green | PASS vacuously | first fixture |
| (c) conformance threshold | PASS vacuously | no suites in phase 00 per SPEC §3 |
| (d) new fuzzer clean | PASS vacuously | no parser/codec in phase 00 |
| (e) vet + lint + test clean | PASS | exit 0 each, locally and in CI (reviewer re-verified build/vet) |
| (f) REVIEW.md approved | PENDING | this document |

All gate evidence was captured with `-count=1` (uncached) runs in the
verification section, so the evidence is not a cache artifact.

## Recommendations (non-blocking)

- For phase 01's writing-plans session: copy the TDD RED/GREEN log format used
  here. It made this review tractable.
- When phase 01 lands the real bootstrap loader, use that phase as the moment
  to clean up the `CompareBytes` `eq`/dead-branch artifact and drop the
  always-nil error from its signature — both are pure cosmetic improvements
  that would churn callers unnecessarily if done mid-phase-00.
- Consider adding a `doc.go` (or repo-root `README.md`) section that points
  new reviewers at `PROGRESS.md`'s RED/GREEN discipline as the canonical
  example. The bar is high and worth preserving.
- When phase 02 retires the placeholder binary, remember to retire the
  `splice(2)` wrapper comment too — it'll be obsolete once the real listener
  manager owns the Copy loops.
- The ADR-numbering gap in `DECISIONS.md` (0008 before 0007) is harmless but
  future-proof the convention by landing ADRs in strict increasing numerical
  order even when a block of numbers is reserved.

## Approval line

I approve phase 00 for advancement to lifecycle-state 6, subject to fixing the
Important findings above (at minimum Finding 1, the ADR-0010 `BEHAVIOR_CONTRACT.md`
note that was promised but never written; Finding 2, the misleading `envoy.yaml`
comment). Findings 3–4 and all Minor items may be deferred to the next phase
without blocking the advancement, because none of them affect correctness of the
differential gate.

## Resolution (fix round, 2026-04-22, post-review)

The conditional approval above is now unconditional. The Important findings
that gated advancement to lifecycle-state 6 were addressed as follows; all fix
work and re-verification land in the same atomic commit as this Resolution
section (see `PROGRESS.md` → *Review fix round* for the verbatim gate outputs).

- **Finding 1 — RESOLVED.** Added `## Test harness host networking` subsection
  to `docs/envoy-go/BEHAVIOR_CONTRACT.md` codifying the
  `dns_lookup_family: V4_ONLY` rule for fixtures using `host.docker.internal`.
  The subsection cites ADR-0010, pins scope to TCP/HTTP/1.1/HTTP/2 on the
  current Envoy pin, defers HTTP/3 / QUIC to a future superseding ADR, and
  captures the host-side `0.0.0.0` bind requirement plus the
  `HostConfigModifier` `extra_hosts` wiring. ADR-0010 itself is unchanged
  (append-only invariant per `BOOTSTRAP_PROMPT.md §4.1` invariant 4); landing
  the promised note satisfies its Consequences clause.

- **Finding 2 — RESOLVED.** Corrected the two `envoy.yaml` comments to describe
  the actual substitution mechanism (`strings.Replace("port_value: 0", ...)` in
  `test/differential/runner_test.go`). Also corrected
  `test/fixtures/0000-tcp-echo/driver/driver.go:26-29`, which carried the same
  "Cmd override indirection" misrepresentation; the reviewer flagged only
  `envoy.yaml` but the doctrine-D-3.4 context-isolation concern extends to the
  sibling godoc. All three comments now match the implementation and the
  already-correct prose at `driver.go:102-105`.

- **Findings 3, 4, and all Minor items — DEFERRED per approval line.** No
  action in this fix round. Findings 3 and 4 are recommended for phase 01/02;
  Minors are nice-to-have. The deferral is explicit in the Approval line above.

**Re-verification.** Because the diff is comment/docs only, the fix round runs
no new Go code. `go build`, `go vet`, `golangci-lint`, `go test -short ./...`,
and the differential suite (`go test -count=1 ./test/differential/... -v`) all
re-ran clean at the fix-round head. Outputs are quoted verbatim in
`PROGRESS.md` under *Review fix round → Re-verification*. Phase-done gate
roll-up is all PASS, including gate (f), which this Resolution section
satisfies.

**Verdict:** APPROVED (unconditional). Phase 00 is cleared to advance to
lifecycle-state 6 → ROADMAP row 00 status `done` → STATE.md's active phase
advances to `01-static-bootstrap-config`.
