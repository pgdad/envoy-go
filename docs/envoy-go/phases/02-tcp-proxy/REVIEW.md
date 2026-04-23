# Phase 02 — TCP Proxy Review

**Reviewer:** superpowers:requesting-code-review (dispatched 2026-04-23)
**Range:** `013daa7..3da655d`
**Verdict:** APPROVED

## Summary

Phase 02 delivers what SPEC §1 promises: the phase-00/01 ad-hoc TCP pump
in `cmd/envoy-go/main.go` is retired and the first real envoy-go
dataplane lands — a listener manager at `internal/listener/`, a cluster
manager with a round-robin LB at `internal/cluster/`, a TCP proxy filter
at `internal/filter/tcpproxy/`, and a rewired `cmd/envoy-go/main.go`
wiring shell that composes them. The pre-existing fixture
`0000-tcp-echo` stays green through the cutover, and the new fixture
`0001-tcp-proxy-rr` lands green with byte-exact response equivalence
over a 3-endpoint STATIC cluster plus the promised per-proxy
distribution assertion `[3, 3, 3]` over 9 requests. Every one of the 10
PLAN tasks lands as an atomic commit traceable through `PROGRESS.md`
with verbatim command outputs, and all seven promised ADRs (ADR-0022
through ADR-0027 mapped to SPEC §4.4 ADRs A–F, plus the
execution-time-discovered ADR-0028) appear in `DECISIONS.md` with
Context / Decision / Rationale / Consequences bodies and cross-references.
The `## Verification` block at the tail of `PROGRESS.md` (commit
`3fc5f15`) quotes all five executable phase-done gates verbatim from a
fresh-shell `go clean -testcache` run on branch tip `af59456`; per the
STATE.md review scope I did not re-run any of those. Reviewer
spot-checks (`go build ./...`, file-existence walk, ADR count grep,
extractor-retirement grep, pump-verbatim diff) were clean: `go build
./...` is exit 0 at `3da655d`, `grep -c '^## ADR-' = 28`, the old
`First*` extractor symbols only appear in tombstone comments + historic
docs (no live code reference), and the `pump` body at
`internal/filter/tcpproxy/filter.go:79-83` is a character-identical
copy of the phase-00 pair at the old `cmd/envoy-go/main.go:112-117`
modulo method-inlining. No finding rises to Critical or Important;
eight Minors, all deferrable to phase 03+. The phase is safe to advance
to lifecycle-state 6 unconditionally.

## Strengths

- **Cluster manager is shape-correct and covers every §5.4 build-time
  rule.** `internal/cluster/manager.go:32-49` walks
  `bs.GetStaticResources().GetClusters()`, the happy path materialises
  via `buildCluster` at lines 57-90, and the build-time errors enumerate
  all six rules from SPEC §4.1 / §5.4: zero clusters (line 35),
  duplicate name (43-45), non-STATIC discovery type (62-68), non-
  ROUND_ROBIN policy (69-71), missing load_assignment (72-75), and zero
  / non-socket-address endpoints (extracted at 92-114). Error messages
  begin with `cluster: ` uniformly per SPEC §7. Each error path has a
  corresponding `TestManager_Error_*` case at
  `internal/cluster/manager_test.go:118-244`.
- **Round-robin LB formula + per-cluster scope is exactly what ADR-0024
  codifies.** `internal/cluster/loadbalancer.go:23-29`:
  `i := rr.counter.Add(1) - 1; return rr.endpoints[int(i)%len(rr.endpoints)]`
  — the subtract-one trick makes the first pick `endpoints[0]`, pinned
  by `TestRoundRobin_FirstPickIsEndpoint0`
  (`loadbalancer_test.go:33-47`). The concurrent-distribution test at
  `loadbalancer_test.go:49-87` exercises 100 goroutines × 30 picks =
  3000 picks and asserts the 1000/1000/1000 split exactly, matching
  SPEC §4.1's claim about `atomic.Uint64.Add(1)` uniqueness.
- **TCP proxy filter's pump is a character-identical lift of phase 00's
  pump, as ADR-0023 promises.** `diff`'ing the old
  `cmd/envoy-go/main.go:91-119` against
  `internal/filter/tcpproxy/filter.go:79-98` shows the `netConn`
  wrapper, the `halfClose` helper, and the two-goroutine `io.Copy` +
  `halfClose` dance are byte-identical; the only differences are the
  splice-avoidance comment's footer (`SPEC §5.3 requires the pump be
  untouched` → `per ADR-0023`) and the removal of the free `pump`
  function (now inlined into `Filter.Handle`'s method body). The verb
  "lifted" in ADR-0023 is honored.
- **Listener manager's six-rule filter-chain gate is complete and
  tested.** `internal/listener/manager.go:98-140` enforces every item
  from ADR-0025: exactly one filter_chain (108-110), empty
  `filter_chain_match` via `proto.Equal` (112-114), nil
  `transport_socket` (115-117), exactly one filter (118-121), registered
  filter `type_url` (126-129), and surfaces constructor errors with
  `listener: %q: %w` wrapping (132). `listener_filters` is silently
  skipped per SPEC §2 (no code reads `l.GetListenerFilters()`; the
  absence is the feature). Unit tests at `manager_test.go:199-431`
  cover each of the six rules plus duplicate name, zero listeners,
  non-socket-address, bind unwind, and the filter-constructor-error
  propagation case that ensures the tcpproxy error string is wrapped
  (`TestManager_Error_FilterConstructionPropagated` at lines 355-373).
- **Cutover atomicity is honored.** Task 7's `1143d8d` is a single
  commit touching 11 files (`cmd/envoy-go/main.go` +
  `main_test.go`, `internal/bootstrap/bootstrap.go` + `bootstrap_test.go`,
  `test/differential/harness.go` + `fixture/fixture.go` +
  `harness_test.go` + `runner_test.go`, `test/fixtures/0000-tcp-echo/driver/driver.go`,
  `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/phases/02-tcp-proxy/PROGRESS.md`)
  — exactly the set PLAN Task 7 "Sub-steps" lists. No silent-stale
  caller of `FirstListenerSocket` / `FirstClusterEndpointSocket`
  survived the cutover; `grep -rn "First\(Listener\|ClusterEndpoint\)Socket" .`
  returns hits only in historic phase-01 docs, PLAN/SPEC references,
  and the tombstone comment at `internal/bootstrap/bootstrap.go:74-77`.
- **cmd/envoy-go/main.go is a wiring shell, as the SPEC §12 first item
  requires.** `grep -n "net\.Listen\|io\.Copy\|pump\|halfClose\|netConn\|Accept" cmd/envoy-go/main.go`
  returns one hit: a comment at line 7 describing the retirement of the
  ad-hoc pump. No `net.Listen`, no `io.Copy`, no `Accept` loop, no
  pump definitions. The startup order at `main.go:33-82` matches
  SPEC §5.2 / §6.1 step-for-step: load → AdminSocket → cluster manager
  → admin.New/Start → listener.NewManager → SIGINT-bound ctx →
  lm.Start → MarkReady → per-listener sentinels → terminal sentinel →
  block on ctx.
- **Ready-sentinel format lands as ADR-0026 specifies.**
  `cmd/envoy-go/main.go:76-79` emits the per-listener lines (via
  `lm.Listeners()` iteration) followed by the terminal `envoy-go ready`
  line. The harness's replacement parser
  `readyListenerAddrs(ctx, r)` at
  `test/differential/harness.go:60-91` walks lines until the terminal
  sentinel, matching the per-listener line via regex
  `^envoy-go listener (\S+) ready on (\S+)$`. The clean-break promise
  holds: `grep -n "envoy-go ready on" .` returns zero hits in live
  code (only historical docs + the phase-01 REVIEW.md reference). The
  `cmd/envoy-go/main_test.go` rewrite at line 25 asserts both per-listener
  + terminal sentinels via the duplicated `waitForReadySentinels`
  helper (intentionally duplicated from the harness to avoid the test
  pulling in the differential subsystem).
- **FixtureDriver interface growth is spec-faithful.**
  `test/differential/fixture/fixture.go:15-58`: the `Driver` interface
  now carries `BackendCount() int`, `SubjectListenerName() string`,
  `ReferenceBootstrap(backendPorts []int) string`,
  `SubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string`,
  and the separate-drive `Drive(ctx, refAddr, subjAddr string)`
  signature is documented to accept `""` for either side, honoring the
  runner's baseline-snapshot-between-drives discipline at
  `runner_test.go:101-127`. The optional `DistributionAsserter` at
  lines 56-58 is a clean additive interface that fixture 0000 ignores
  and fixture 0001 implements — the type-assertion pattern at
  `runner_test.go:140-144` is idiomatic.
- **Fuzz target is structurally minimal and hits every NewFilter error
  path.** `internal/filter/tcpproxy/fuzz_test.go:26-56`: the three seed
  corpus entries (well-formed TcpProxy, wrong type_url, malformed proto
  bytes) seed each distinct return path in `NewFilter`. The fuzz body
  asserts "no panic, and every error starts with `tcpproxy: `",
  matching the SPEC §7 prefix discipline. The 30s CI budget
  (ADR-0018 inherited) is honored by Task 10's verification run:
  `5,494,050 execs` at 31.051s with `PASS` and no new crashes
  (`PROGRESS.md:487-502`).
- **BEHAVIOR_CONTRACT `## TCP proxy` subsection is substantive.**
  `docs/envoy-go/BEHAVIOR_CONTRACT.md:133-172` adds the H2 section
  promised by SPEC §4.4 Task 8 with four concrete rules: response-body
  byte-equivalence (asserted), half-close propagation (asserted), LB
  endpoint-selection sequence (NOT asserted, with a two-bullet
  rationale citing per-worker randomized offset), and listener-bind
  error semantics (asserted with the `log.Fatalf` parity note). The
  "Applies to" / "Does not yet apply to" footer enumerates phase-03+
  deferrals, matching the phase-01 subsection's shape exactly.
- **Every §10 SPEC deferred decision is settled with evidence.** PLAN
  §"Settled SPEC §10 deferred decisions" (lines 88-100) walks all nine;
  the ones that landed as code match PLAN text verbatim:
  flat subpackage layout (#1), 5s `defaultConnectTimeout` at
  `cluster.go:11` (#2), `listener.Manager.Stop` implemented + tested
  at `manager.go:211-221` + `manager_test.go:139-145` (#3), 3 fuzz
  seeds at `fuzz_test.go:26-42` (#4), clean-break sentinel format
  (#5, ADR-0026), fixture id `0001-tcp-proxy-rr` (#6), shared-ctx
  across Accept loops at `manager.go:150-175` (#7), `statPrefix` stored
  on the Filter struct (`filter.go:26`) (#8), ADR numbering
  0022–0027 (#9).
- **Unexpected reality check caught two real bugs before the gate
  and ADR-0028'd them.** Task 10's first differential run (per
  PROGRESS.md §Task 10) failed because (a) the runner's
  per-side-Drive pattern (Task 7 refactor) combined with both fixture
  drivers' `randHex(6)` per-call payload uid produced divergent
  payloads between the ref-side and subj-side calls; (b) upstream
  Envoy's default multi-worker RR LB produced `[5, 3, 1]` distribution
  instead of `[3, 3, 3]`. The `40464d2` fix makes the driver payloads
  deterministic and pins the reference container's command-line with
  `--concurrency 1`, preserving SPEC §5.8's per-proxy exact-distribution
  assertion. ADR-0028 at `DECISIONS.md:831-875` documents both fixes
  in one ADR with option-tree rationale (Option A `--concurrency 1` vs
  Option B subject-only assertion, A chosen because it preserves
  SPEC without edit) and a Consequences section that explicitly calls
  out the payload determinism as a Task-7-fallout bug repair.

## Findings

### Critical (must fix before lifecycle-state 6)

*None.*

### Important (should fix, reviewer's judgment call)

*None.*

### Minor (nice-to-have, can defer)

1. **ADR-0028 was unanticipated by SPEC §4.4 and PLAN §"ADRs introduced
   by this plan".** `docs/envoy-go/DECISIONS.md:831-875` lands a
   seventh phase-02 ADR covering the reference `--concurrency 1` pin
   and the deterministic-payload fix. SPEC §4.4 lists only ADRs A–F
   (0022–0027), and PLAN line 75 pre-registers exactly those six;
   PLAN line 84 permits "ADR-0028+" for unforeseen cross-phase decisions
   but framed that as a general escape valve, not a spec gap. The root
   cause is that SPEC §5.8's "3/3/3 exact" assertion implicitly assumed
   single-worker reference semantics without saying so, and SPEC §5.4
   didn't think to describe upstream's per-worker randomized starting
   offset as a concrete reconciliation obstacle for the *distribution*
   (rather than just the *sequence*) assertion. PROGRESS.md Task 10
   calls this out honestly ("Gate-run exposed two real bugs in the
   Task 7 cutover"); ADR-0028's Rationale section walks Option A vs
   Option B and picks A. No action required — the ADR exists, the
   fix is committed, the gate is green. Recommend phase 03's SPEC
   author scrutinize any fixture's "exact-on-both-sides" claim for
   underlying reference-side worker-count assumptions before the
   planner drafts PLAN.md.

2. **ADR-0028's Consequences section bundles an unrelated bug repair
   (the deterministic-payload fix) rather than splitting it into its
   own ADR or a non-ADR'd PROGRESS note.**
   `docs/envoy-go/DECISIONS.md:866` reads "Unrelated phase-02 fix: the
   `randHex(6)` per-call uid in both fixture drivers' `Drive` methods
   was removed at the same time." The fix is a correctness bug in the
   Task 7 runner refactor (per-side Drive calls don't share state,
   so per-call randomness diverges across calls), not a cross-session
   decision per D-3.5. Two cleaner options existed: (a) a standalone
   ADR-0029 or (b) no ADR at all (bug fixes don't need ADRs). The
   chosen option — burying the fix in an unrelated ADR's Consequences
   — makes a future reviewer searching for "why did driver payloads
   change" land on ADR-0028 whose title is about `--concurrency`.
   Fix (deferrable): when phase 03's SPEC author references this
   pattern, note that bug-fix-ADRs are anti-pattern; decisions are
   ADR'd, bugs are commit-messaged. Not a doctrine violation because
   D-3.5 says decisions MUST be written, not that only decisions may
   be written; the Consequences paragraph is a valid place for adjacent
   fixup notes.

3. **ADR numbering still drifts from physical-file order, worsening the
   phase-01 Minor 2 complaint.** `grep -n '^## ADR-' DECISIONS.md`
   returns (numbers-by-line-order): 0001..0006, 0008, 0007, 0009, 0010,
   0011, 0013, 0012, 0016, 0017, 0018, 0015, 0014, 0019, 0020, 0021,
   **0024, 0023, 0025, 0022, 0026, 0027, 0028**. Phase 02 appends
   0024 before 0022 because Task 2 (ADR-0024) commits before Task 7
   (ADR-0022). This is the "append at commit time, not in numerical
   order" pattern phase-01 REVIEW's Minor 2 flagged. Per
   `BOOTSTRAP_PROMPT.md` §4.1 invariant 4 the *numbering* is the
   authoritative ordering; physical position is not. So this is
   compliant, but tail-of-file navigation remains tedious. Fix
   (deferrable, inherits from phase 01): if a future phase appends an
   ADR, consider a one-line mechanical pass that sorts the ADRs by
   number at append time (append-only still holds — the file's last
   write appends the new ADR at file tail, then a second append-only
   move within the same commit reorders by number). No action required
   in phase 02.

4. **`Filter.Handle`'s `ctx context.Context` parameter is accepted but
   never consulted after the upstream dial.**
   `internal/filter/tcpproxy/filter.go:65-84`: `ctx` is passed as the
   first method parameter and used only indirectly via
   `net.DialTimeout` (which takes a duration, not a ctx). The two
   `io.Copy` goroutines at lines 81-82 run to completion regardless of
   ctx cancellation — an outstanding connection after `lm.Stop()` or
   SIGINT is not actively terminated; it drains when the peer closes
   or the read returns EOF. SPEC §7 "`tcpproxy.Filter.Handle`: pump
   error (read/write)" describes exactly this: "Silently dropped by
   `_ = io.Copy(...)` (same as phase 00). Both halves run to completion."
   So the behaviour is SPEC-conforming. The cosmetic issue is that
   accepting `ctx` without reading it signals to a future reader that
   cancellation would be honored, when it is not. Fix (deferrable,
   phase 08 per the drain deferral): wrap the pump goroutines with a
   `ctx.Done()` side-channel that sets a read deadline on the
   downstream/upstream conns so `io.Copy` returns. Or — if drain
   truly belongs in phase 08 — rename the parameter to `_ context.Context`
   to surface the intentional unused-ness, with a doc-comment pointing
   at SPEC §7 / phase-08. Not phase-02-blocking.

5. **`readyListenerAddrs` spawns a goroutine that can leak if ctx is
   canceled before the subprocess emits the terminal sentinel.**
   `test/differential/harness.go:60-91`: the inner goroutine reads
   from `r` (the subject's stdout pipe) until it sees `envoy-go ready`
   or an `err` from `ReadString`. If `ctx.Done()` fires first (line
   88-89), the select returns with `ctx.Err()` but the goroutine
   continues reading; it exits only when the pipe closes or it hits
   the terminal sentinel. `StartSubjectProxy` (line 214-220) kills the
   subprocess on that path, which closes the pipe and frees the
   goroutine — so the leak is bounded by subprocess lifetime, not
   unbounded. No current defect. Fix (deferrable): wrap the
   `br.ReadString('\n')` loop with a per-iteration `select { ctx.Done()
   || continue }`, or defer-close a cancellation channel the reader
   polls. Not worth fixing in phase 02 because the path is only hit
   when a subject fails to emit the sentinel within `readyTimeout`,
   which itself triggers the `cmd.Process.Kill()` that drains the
   pipe.

6. **The separate-per-side `Drive` contract is documented only in a
   comment, not enforced by the interface.**
   `test/differential/fixture/fixture.go:45-46`: "Runners may pass ""
   for either side to signal 'don't drive this side'; drivers must
   no-op the corresponding side in that case." A driver that doesn't
   honor `""` — e.g., a naive `helpers.TCPRoundTrip(ctx, "", …)` call
   — would fail at dial time with a DNS lookup on the empty string.
   Both fixture drivers (0000 at `driver.go:107-118`, 0001 at
   `driver.go:109-131`) honor the convention via early `if refAddr
   != ""` / `if subjAddr != ""` guards. Fix (deferrable): either
   (a) split `Drive` into two methods (`DriveReference(ctx, addr)` +
   `DriveSubject(ctx, addr)`), or (b) introduce a typed sentinel like
   `fixture.SkipSide` so a forgotten guard is a compile error. Option
   (a) is cleaner and aligns with the runner's already-separate
   invocation at `runner_test.go:107` and `runner_test.go:121`.
   Option (b) preserves the single-method surface. Neither is
   phase-02-blocking; defer to whichever phase next adds a fixture.

7. **Fixture 0001's `expectations.yaml` carries a significant amount of
   specification as comments instead of structured entries.**
   `test/fixtures/0001-tcp-proxy-rr/expectations.yaml:1-18` has 13
   lines of comments describing what the fixture checks (admin
   /ready, distribution assertion, N/A dimensions) versus 3 lines of
   actual YAML (`response-body: applicable: true scope: byte-exact`).
   Per phase-01 ADR-0019 the `expectations.yaml` is a forward-looking
   dimension-aware-diff artefact; phase 02's runner doesn't consume it
   (it reads hard-coded allow-lists from `runner_test.go:186-190`), so
   the YAML is effectively documentation. This is consistent with
   fixture 0000's `expectations.yaml` which also has more prose than
   data. Fix (deferrable, phase 06/08 per ADR-0019): when the runner
   starts consuming the YAML, each fixture's comments should convert
   to actual entries (e.g., `admin-ready: { applicable: true, scope:
   byte-exact-with-allowlist: [Date, Content-Length,
   Transfer-Encoding] }`). Not a phase-02 defect.

8. **The `## TCP proxy` BEHAVIOR_CONTRACT subsection doesn't cross-link
   to ADR-0028, even though `--concurrency 1` is a load-bearing part of
   the distribution-equivalence claim.**
   `docs/envoy-go/BEHAVIOR_CONTRACT.md:145-153`: the "Load-balancer
   endpoint-selection sequence (NOT asserted)" section names ADR-0024
   (subject-side per-cluster counter) and SPEC §5.4 (sequence-starts-
   at-0). It does NOT name ADR-0028 (reference-side single-worker
   pinning), which is what actually makes the reference side's
   distribution exact-3/3/3. A stranger reading the BEHAVIOR_CONTRACT
   in isolation would conclude that "upstream Envoy's RR LB is
   per-worker-thread with a randomized starting offset" contradicts
   the fixture's distribution assertion — resolved only by reading
   ADR-0028 separately. Fix (deferrable): when a phase next edits
   `## TCP proxy`, append a sentence to the "Load-balancer
   endpoint-selection sequence" paragraph: "*Reference-side
   distribution exactness is achieved by pinning `--concurrency 1` on
   the reference container per ADR-0028.*" The BEHAVIOR_CONTRACT
   then reads coherently on its own per D-3.4 context-isolation. Not
   phase-02-blocking because ADR-0028's Cross-references section
   (`DECISIONS.md:873`) names the BEHAVIOR_CONTRACT subsection in the
   reverse direction, so the two-link chain is discoverable.

## Axis-by-axis assessment

### Axis 1: PLAN.md fidelity

Walked all 10 tasks against the diff and PROGRESS.md. Every task
produced the expected code/doc and an atomic commit with a matching
PROGRESS.md entry. Deviations from PLAN text were either
PROGRESS-documented (Task 6's `acceptLoop` signature took an explicit
`net.Listener` arg to avoid a nil-pointer race — documented in
PROGRESS §Task 6 Notes; `ListenerInfo` → `Info` rename for revive's
stutter lint — same; British → US spelling conversions throughout —
same), ADR'd (Task 10's payload-determinism + `--concurrency 1` →
ADR-0028), or trivially absorbed (Task 9's `helpers.HTTPGetReadyRaw`
extraction from fixture 0000's old `probeReady` — PLAN anticipated
this).

| Task | PLAN intent | Commit | PROGRESS | ADR for deviation | Verdict |
|---|---|---|---|---|---|
| 1 | PROGRESS preamble + precondition verify | `b6410ca` | yes, verbatim | — | pass |
| 2 | `internal/cluster` Cluster + Endpoint + RR LB | `24a6668` | yes, RED+GREEN | ADR-0024 | pass |
| 3 | `internal/cluster.Manager` build-time materialisation | `958c059` | yes, RED+GREEN | — | pass |
| 4 | `internal/filter/tcpproxy` Filter + NewFilter + Handle | `aa9b43f` | yes, RED+GREEN | ADR-0023 | pass |
| 5 | `FuzzTcpProxyFilter` (gate d) | `e01161e` | yes, 30s fuzz output quoted | — | pass |
| 6 | `internal/listener.Manager` multi-listener build + Start/Stop | `4151926` | yes, 12 unit tests | ADR-0025 | pass |
| 7 | Cutover (atomic) | `1143d8d` | yes, 11-file commit stat verified | ADR-0022, ADR-0026 | pass |
| 8 | BEHAVIOR_CONTRACT `## TCP proxy` | `de2f06e` | yes | — | pass |
| 9 | Fixture `0001-tcp-proxy-rr` + AssertDistribution | `9fc9be8` | yes, 4 driver-unit tests | ADR-0027 | pass |
| 10 | All-gates green run | `40464d2` (fix) + `1a2cf90` (green run) | yes, every gate quoted verbatim | ADR-0028 | pass |

No task silently skipped. No scope-creep introduced. Task 10's
split into a fix commit (`40464d2`) and a PROGRESS-green-run commit
(`1a2cf90`) is cleaner than lumping into one — the fix ADR-0028 lives
with the code it decides for.

### Axis 2: SPEC.md §12 acceptance checklist

| Item | Status | Evidence |
|---|---|---|
| `cmd/envoy-go/main.go` contains no direct `net.Listen`, `Accept`, `io.Copy`, or pump helpers | PASS | `grep -n "net\.Listen\|Accept\|io\.Copy\|pump\|halfClose\|netConn" cmd/envoy-go/main.go` returns one hit, a doc-comment at line 7 about the retirement. `main.go` is 82 lines of pure wiring. |
| `internal/listener/manager.go` exists + builds + unit tests pass | PASS | 221 lines; PROGRESS Task 6 quotes 12 passing unit tests. Build-time errors match §7. |
| `internal/cluster/manager.go` + `cluster.go` + `loadbalancer.go` exist + unit tests pass | PASS | 115 + 51 + 29 lines; PROGRESS Task 3 quotes 15 passing unit tests (4 RR + 11 Manager). |
| `internal/filter/tcpproxy/filter.go` exists; pump verbatim per ADR-0023 | PASS | 98 lines; reviewer `diff` of old `cmd/envoy-go/main.go:91-119` vs new `filter.go:86-98` confirms character-identical netConn/halfClose/pump bodies modulo the free `pump` function being inlined into `Handle`. |
| `internal/filter/tcpproxy/fuzz_test.go` exists; `FuzzTcpProxyFilter` runs clean on CI short budget | PASS | 56 lines; PROGRESS Task 5 + Task 10 + §Verification all quote 30s clean runs. |
| `internal/bootstrap.FirstListenerSocket` + `FirstClusterEndpointSocket` deleted | PASS | `grep -rn "First\(Listener\|ClusterEndpoint\)Socket"` in live code returns only a tombstone comment at `internal/bootstrap/bootstrap.go:74-77`. `Load` and `AdminSocket` unchanged (verified by reading `bootstrap.go:26-72`). |
| `test/differential/harness.go` parses per-listener sentinels; `ListenerAddr(name)` accessor | PASS | `harness.go:60-91` (`readyListenerAddrs`) + `harness.go:224-227` (`SubjectProxy.ListenerAddr(name)`). Old `readyAddr` parser deleted. |
| Fixture `0000-tcp-echo/driver/driver.go` calls new `ListenerAddr("l_tcp")` form | PASS (transitively) | Fixture-0000 driver at `driver.go:22` returns `"l_tcp"` via `SubjectListenerName()`; the runner's `runner_test.go:121` consumes it via `subj.ListenerAddr(d.SubjectListenerName())`. The driver itself does not call `ListenerAddr` directly — the runner does, with the driver-supplied name. |
| Fixture `0001-tcp-proxy-rr/` exists with all six files | PASS | `envoy.yaml` (34 lines), `envoy-go.yaml` (28), `expectations.yaml` (18), `README.md` (28), `driver/driver.go` (170), `driver/driver_test.go` (34) — all present per `ls`. |
| `BEHAVIOR_CONTRACT.md` contains populated `## TCP proxy` section | PASS | Lines 133-172, four subsections: response-body byte-equivalence, half-close propagation, LB sequence NOT asserted with rationale, listener-bind error semantics. Cites ADR-0023 + ADR-0024. |
| `DECISIONS.md` contains ADRs 0022–0027 | PASS + one extra | ADR-0022 (line 706), ADR-0023 (627), ADR-0024 (589), ADR-0025 (660), ADR-0026 (748), ADR-0027 (792) — all six present with Status / Date / Doctrine headers and Context / Decision / Rationale / Consequences bodies. ADR-0028 (line 831) landed at execution time per the PLAN-allowed escape valve (see Minor 1). |
| ROADMAP.md row for phase 02 `done` at commit | PARTIAL (expected, per lifecycle, same as phase-01) | `STATE.md` is at `lifecycle-state: 5` (this review satisfies it); `ROADMAP.md` row 02 still shows `in-progress`. Per SPEC §3 and the state machine, the advance to `done` is the next-session action *after* this REVIEW lands approved — same treatment phase-01's REVIEW applied. |
| `STATE.md` advances to phase 03 at commit | NOT YET (expected — review-session scope is state 5→6, not state 6→next-phase) | Same pattern as phase-01. |
| `go build`, `go vet`, `golangci-lint`, `go test ./...` clean | PASS (per PROGRESS §Verification) | `3fc5f15` captures all four exit 0 after `go clean -testcache`. Reviewer sanity re-ran `go build ./...` (clean exit) — not a heavy-gate re-run, just a dependency-resolution check at `3da655d`. |
| Commit message follows `BOOTSTRAP_PROMPT.md` §5.3 format | PASS | Task 2 `[ADR-0024]`, Task 4 `[ADR-0023]`, Task 6 `[ADR-0025]`, Task 7 `[ADR-0022, ADR-0026]`, Task 9 `[ADR-0027]`, Task 10 fix `[ADR-0028]` — each ADR-bearing commit names the ADRs it lands. |
| `REVIEW.md` approved | IN FLIGHT | This document. |

### Axis 3: Doctrine D-3.1–D-3.7

| Doctrine | Status | Evidence |
|---|---|---|
| D-3.1 Superpowers-first process | PASS | Brainstorming seeded SPEC.md; writing-plans produced PLAN.md; TDD evidence in PROGRESS task-by-task (RED compile failures in Task 2 — `undefined: roundRobin`; Task 3 — `undefined: NewManager`; Task 4 — tests fail before implementation; Task 6 — same); verification-before-completion section at PROGRESS tail; this REVIEW.md is the requesting-code-review output. Unexpected-state events in Task 10 (`[5, 3, 1]` distribution, uid-divergence) resolved via systematic-debugging → ADR-0028. |
| D-3.2 Hybrid implementation stance | PASS | Permitted foundations only: `go-control-plane/envoy` proto types (config/{bootstrap,cluster,endpoint,listener,core}/v3, extensions/filters/network/tcp_proxy/v3 — grep-verified by import lists in `cluster/manager.go`, `listener/manager.go`, `filter/tcpproxy/filter.go`), `google.golang.org/protobuf/types/known/{anypb,durationpb,wrapperspb}`, `google.golang.org/protobuf/proto` (for `proto.Equal` in the filter-chain-match zero-check), stdlib (`net`, `io`, `sync`, `sync/atomic`, `context`, `log`, `time`). No `httputil.ReverseProxy`, no Traefik/Caddy/fasthttp, no cgo, no GPL. Cluster manager imports proto types only; no `pkg/...` helpers. |
| D-3.3 Differential correctness beats internal fidelity | PASS | Fixture `0001-tcp-proxy-rr` byte-compares upstream Envoy v1.37.2 against the envoy-go subprocess on concatenated response bodies of 9 TCP round-trips through a 3-endpoint cluster. Fixture `0000-tcp-echo` byte-compares the phase-01 TCP echo surface + admin `/ready` (no regression). The per-proxy distribution-exact-3/3/3 assertion is a LOCAL correctness property of each proxy independently — deliberately not a differential dimension per the new BEHAVIOR_CONTRACT subsection. Reviewer verified no mocks on either surface (both proxies are real binaries with real sockets). |
| D-3.4 Context isolation | PASS (with Minor 8 reservation) | SPEC, PLAN, PROGRESS, ADRs are stranger-readable. ADR-0028 (Minor 2) bundles a bug fix under a `--concurrency` ADR, mildly weakening local readability of the Consequences section — but the ADR's title + Context accurately frame the primary decision. BEHAVIOR_CONTRACT's `## TCP proxy` subsection reads coherently on its own *except* for the Minor-8 omission of a cross-reference to ADR-0028 from the "LB sequence NOT asserted" paragraph — a stranger reading only the subsection would think upstream's per-worker RR contradicts the fixture's distribution assertion. The contradiction resolves via ADR-0028 but isn't pointer-discoverable from the BEHAVIOR_CONTRACT side. Every other cross-file note promised by an ADR is actually in place: ADR-0022's `FirstListenerSocket` retirement effected (tombstone comment at `bootstrap.go:74`); ADR-0023's pump lift verifiable by `git diff`; ADR-0024's per-cluster scope rendered in `loadbalancer.go`; ADR-0025's six-rule gate rendered in `listener/manager.go`; ADR-0026's sentinel format rendered in `cmd/envoy-go/main.go` + `harness.go`; ADR-0027's STRICT_DNS/STATIC divergence rendered in fixture 0001 yaml pair. |
| D-3.5 Decisions are written, not remembered | PASS | Seven new ADRs (0022–0028) in-session. ADR append-only discipline maintained — no landed ADR edited in place (PROGRESS verifies append-only commit shape for each ADR landing). ADR-0026 names its supersession target via `**Supersedes:** (informal) phase-00 sentinel contract encoded in cmd/envoy-go/main.go:79 comment and test/differential/harness.go:readyAddr` — the `(informal)` qualifier is appropriate because the prior contract was never ADR'd. Phase-01 REVIEW's Minor 2 numbering-order complaint persists in phase 02 (see Minor 3), but this is a D-3.5 append-at-tail choice, not a D-3.5 append-only violation. |
| D-3.6 Every phase is a green build | PASS | All five executable phase-done gates exit 0 at the verification head per PROGRESS §Verification. Reviewer sanity check: `go build ./...` clean at `3da655d`. The verification-session `go clean -testcache` fresh-shell re-run at `af59456` (PROGRESS lines 428-563) records all four command groups `[exit=0]` — build, vet, lint, test — plus both fuzz runs + the differential suite. |
| D-3.7 Version pinning | PASS | Envoy: tag `v1.37.2` + SHA256 `c5e8a68e…` (unchanged from phase 00/01; verified visible in the `TestDifferential` logs at PROGRESS line 547 and harness.go image ref). Go: `go 1.23.0` in `go.mod` (unchanged; toolchain runs 1.26.2 per `go version` in PROGRESS Task 1). golangci-lint: `v1.64.8` (unchanged). NEW pins: none — phase 02 adds no new dependencies beyond what phase 01's go.mod already declares (the `go-control-plane/envoy` proto-type pin at `v1.32.4` covers all new imports: `config/cluster/v3`, `config/endpoint/v3`, `config/listener/v3`, `extensions/filters/network/tcp_proxy/v3` — all part of the same nested module). Reference container `--concurrency 1` pin is ADR-0028. |

### Axis 4: Phase-done gate (SPEC §3 / BOOTSTRAP_PROMPT §7.5)

Per STATE.md review scope, the heavy gates were NOT re-run; the
`## Verification` block at `PROGRESS.md:400-579` is cited as evidence.
Gate-by-gate, verbatim from that block:

| Gate | Verbatim PROGRESS evidence | Verdict |
|---|---|---|
| (a) new fixture `0001-tcp-proxy-rr` green | `--- PASS: TestDifferential/0001-tcp-proxy-rr (1.11s)` under `go clean -testcache && go test ./test/differential/ -v -timeout=10m`. Byte-exact response-body plus AssertDistribution exact-[3,3,3] per-proxy (enforced inside the test body via the `DistributionAsserter` type-assert at `runner_test.go:140-144`; no failure reported) | PASS |
| (b) pre-existing fixture `0000-tcp-echo` still green | `--- PASS: TestDifferential/0000-tcp-echo (1.10s)` in the same invocation — no regression under the new dataplane | PASS |
| (c) conformance threshold | N/A per SPEC §3 row (c) (h2spec/h3spec/grpc all later phases) | vacuously green |
| (d) new fuzzer clean short-budget | `FuzzTcpProxyFilter` ended `PASS`, `ok …/internal/filter/tcpproxy 31.051s`, 5,494,050 execs, 60 new interesting, no crashes. Phase-01 `FuzzBootstrapLoad` also clean: `PASS`, `ok …/internal/bootstrap 31.079s`, 1,138,604 execs, 70 new interesting, no crashes. | PASS |
| (e) vet + lint + test clean | `go vet ./...` `[exit=0]` empty; `golangci-lint run ./...` `[exit=0]` empty; `go test ./... -timeout 10m` all `ok`, zero `FAIL`; `go build ./...` `[exit=0]` | PASS |
| (f) REVIEW.md approved | this document | IN FLIGHT → PASS on commit |

Cheap reviewer sanity checks (not a re-run of the heavy gates):

- `go build ./...` at `3da655d`: exit 0, no output.
- File-existence walk over SPEC §4.1 / §4.2 / §4.3 paths: every
  created file present; both deleted extractor symbols absent (grep
  returns no live-code hits).
- ADR presence grep: `grep -c '^## ADR-' docs/envoy-go/DECISIONS.md`
  returns `28`, matching the expected ADR-0001..ADR-0028 range.
- Supersession grep: `**Supersedes:** (informal)` appears exactly
  once at ADR-0026's `DECISIONS.md:753`. No landed ADR was edited
  (no `**Supersedes:** ADR-00NN` header besides ADR-0021's existing
  one from phase 01).
- Pump-verbatim-lift check: reviewer `git show 24a6668:cmd/envoy-go/main.go`
  vs the current `internal/filter/tcpproxy/filter.go` confirmed the
  netConn + halfClose + two-goroutine-pump bytes are identical, modulo
  the free `pump` function becoming `Filter.Handle` (method receiver +
  `net.DialTimeout` replacing `net.Dial` + one comment footer edit).
- Tombstone comment check: `internal/bootstrap/bootstrap.go:74-77`
  carries a four-line comment pointing `FirstListenerSocket` /
  `FirstClusterEndpointSocket` readers at ADR-0022. Self-contained;
  no code obligations.

## Recommendations (non-blocking)

- Phase 03's SPEC author: scrutinize any fixture's "exact-on-both-sides"
  claim for underlying reference-side worker-count assumptions before
  the planner drafts PLAN.md (Minor 1). The ADR-0028 fix was a clean
  recovery, but the root cause was a SPEC §5.8 assumption that went
  uninspected at SPEC-write time.
- When a future ADR bundles a bug fix in its Consequences section
  (Minor 2), consider promoting the fix to a standalone ADR or — if
  the fix is genuinely incidental and doesn't decide anything —
  omitting the ADR mention entirely and relying on the commit message.
  ADR-0028's deterministic-payload fix fits the "genuinely incidental"
  bucket; naming it in an ADR at all is borderline.
- The ADR-number-vs-file-order drift (Minor 3) is now three phases
  deep. Strict append-in-increasing-number order is invariant 4 in
  spirit; recommend phase 03 try the "append at commit time, then
  within the same commit move the new ADR to the correct numerical
  position" two-step, landing the ADR always at the file's
  number-tail. Preserves append-only and numeric ordering both. If
  this is too much friction, just commit to "physical order drifts;
  numerical order is authoritative; don't waste review bandwidth on
  this" and add a note to BOOTSTRAP_PROMPT.md so reviewers stop
  flagging it.
- Split the `Drive` interface (Minor 6) when the next fixture lands.
  The `""`-sentinel is a convention that a forgotten guard turns into
  a DNS-lookup failure at runtime. Either split into `DriveReference`
  + `DriveSubject` or introduce a typed sentinel; either is a cleaner
  contract.
- The `readyListenerAddrs` goroutine (Minor 5) could adopt a simple
  `br := bufio.NewReader(r); for { … }` loop that selects on
  `ctx.Done()` per line. Not worth a standalone cleanup in phase 02.
- When a phase next edits `## TCP proxy` in BEHAVIOR_CONTRACT (Minor
  8), add the one-sentence cross-reference to ADR-0028 in the
  "Load-balancer endpoint-selection sequence" paragraph so the
  subsection reads coherently on its own.
- Consider promoting `helpers.HTTPGetReadyRaw` to a named fixture-
  utility module if a phase adds more admin-probe variants. Today it
  lives as a single function alongside `ParseHTTPResponse` and both
  fixture drivers call it; fine for now.

## Approval line

I approve phase 02 for advancement to lifecycle-state 6
unconditionally. The eight Minor findings are deferrable to phase
03+; none affects correctness of the differential gate, the
byte-exactness of the TCP proxy dataplane, the per-proxy distribution
assertion, or the append-only integrity of the ADR log. The phase's
exit criteria (SPEC §3 gates a–e green at the verification head;
BEHAVIOR_CONTRACT `## TCP proxy` subsection populated; ADR-0022 through
ADR-0027 mapped 1:1 to SPEC §4.4 ADRs A–F; cutover atomic in a single
commit; `cmd/envoy-go/main.go` reduced to a wiring shell with no
direct `net.Listen`/`Accept`/`io.Copy`; fixture `0000-tcp-echo` green
under the new dataplane; fixture `0001-tcp-proxy-rr` green on
byte-exact + per-proxy distribution; fuzz gate clean at the ADR-0018
budget) are all met.

**Verdict:** APPROVED.
