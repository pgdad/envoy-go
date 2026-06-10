# Fix Failing GitHub Actions CI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the `ci` workflow (lint-vet-test, differential, fuzz-bootstrap) pass reliably on GitHub Actions, where it currently fails on every push.

**Architecture:** The failures are environment mismatches between the developer machine (Docker Desktop for Linux) and GitHub's runners (plain rootful Docker on 2-core VMs), plus a handful of timing-sensitive unit tests. Fixes are confined to the differential test harness, six fixture drivers, and `.github/workflows/ci.yml` — no production code changes are expected unless Task 5's flake investigation proves a real race in `internal/filter/http/wasm`.

**Tech Stack:** Go 1.23, testcontainers-go, Docker, GitHub Actions, `gh` CLI.

---

## Root-cause inventory (evidence from runs 27262760147, 27244893146, 27244417870, 27235336031, 27197903700)

CI has been red for the entire visible history (331 failures / 8 successes in the last ~340 runs). There are five independent causes:

| # | Symptom in CI | Root cause |
|---|---------------|-----------|
| A | `differential` job: subtests 0019-http-jwt-authn, 0020-http-ext-authz-http, 0021-http-ext-authz-grpc, 0022-http-ext-proc-grpc, 0023-http-ext-proc-body, 0032-http-ratelimit fail deterministically — the **reference** Envoy denies/errors scenarios the subject passes (401/403/500 vs 200, missing 429) | The host-side auxiliary services these fixtures spawn (JWKS server, ext-authz servers, ext-proc servers, ratelimit gRPC service) bind to `127.0.0.1`. The reference Envoy container reaches the host via `host.docker.internal:host-gateway` (test/differential/harness.go:121), which on plain Linux Docker resolves to the bridge gateway IP (e.g. 172.17.0.1) — that path **cannot** reach loopback-bound host sockets, so the ref proxy treats every aux service as down. It passes locally only because Docker Desktop's `host.docker.internal` forwards to the host loopback. The echo *backends* bind `0.0.0.0` (runner_test.go:164) which is why plain proxying subtests pass on CI. |
| B | `differential` job: 0006-access-log fails — ref container exits: `unable to open file '/tmp/envoy-access.log': Permission denied` | NOT a file-mode problem: the runner already pre-creates the host file AND explicitly chmods it to 0666 (runner_test.go:966-973 and :1840-1847), and that code was present in today's failing run (headSha bce2977). Primary hypothesis (plan review finding): the in-container path is `/tmp/envoy-access.log` (`refContainerLogPath`, 0006 driver/driver.go:52). `/tmp` in the envoy image is sticky and world-writable; the bind-mounted file is owned by the host runner uid (1001); Envoy opens it as uid 101 with `O_CREAT`. Ubuntu's default `fs.protected_regular=2` sysctl rejects exactly that open with EACCES (existing file in a sticky world-writable dir, owned by neither the opener nor the dir owner) — file mode is irrelevant. Docker Desktop remaps bind-mount ownership in its VM, which is why it passes locally. Fix candidate: move the container log path out of `/tmp`. |
| C | `differential` job: 0036-http-wasm-body-and-advanced fails intermittently — `listener "l_test_d": bind 0.0.0.0:45750: address already in use`, then `subj start: subject ready: EOF` | Subject listener ports are allocated with the alloc-close-reuse pattern (`freeTCPPort`, defined at harness_test.go:158, called at runner_test.go:987) and multi-listener fixtures derive extra ports as `subjListenerPort+1..+3` (e.g. 0023 driver.go:500-502) without reserving them. Under CI's port churn (testcontainers + per-test backends) a derived port is sometimes already taken. |
| D | `lint-vet-test` job: `go test -short ./...` fails intermittently, each time on a *different* timing-sensitive test: `TestFilter_RootVM_SharedAcrossStreams_NoCrossStreamLeak`, `TestFilter_ConcurrentStreams_NoSharedState` (internal/filter/http/wasm/dispatch_test.go), `TestWatcher_Start_ObservesAtomicRename` (internal/sdsfile/sdsfile_test.go) | Concurrency/filesystem-timing flakes on GitHub's 2-core runners. Needs reproduction before any fix (superpowers:systematic-debugging — no fix without root cause). |
| E | Annotation on every run: Node.js 20 actions deprecated; **actions will be forced to Node 24 from 2026-06-16** (six days from this plan's date) | `.github/workflows/ci.yml` pins actions/checkout@v4, actions/setup-go@v5, golangci/golangci-lint-action@v6 — all Node 20. Not yet fatal, but becomes a forced behavior change mid-June. |

Also observed once (run 27244417870): 0017-http-bandwidth-limit failed. It passed in the latest run; treat as a **watch item** during Task 7 verification, not a task — if it recurs, open a separate systematic-debugging investigation.

**Verification constraint that shapes this plan:** causes A and B cannot be reproduced on the local Docker Desktop machine — local runs pass both before and after the fix. The red→green check for them is CI itself: the workflow's `on: push` trigger runs on any branch, so each task pushes to the work branch and checks the run with `gh`. Local runs still guard against regressions.

---

### Task 1: Set up work branch

**Files:** none (git only)

- [ ] **Step 1: Create and switch to a branch**

```bash
cd /home/esa/git/envoy-go2
git checkout -b fix/ci-github-actions
```

- [ ] **Step 2: Confirm a baseline red run exists** (no need to push an empty commit — use the latest master run as baseline)

```bash
gh run list --branch master --limit 1
```

Expected: latest run conclusion `failure`.

---

### Task 2: Bind ref-facing aux services to 0.0.0.0 (fixes A — six subtests)

**Files:**
- Modify: `test/fixtures/0019-http-jwt-authn/inputs/driver.go:218`
- Modify: `test/fixtures/0020-http-ext-authz-http/inputs/driver.go:233`
- Modify: `test/fixtures/0021-http-ext-authz-grpc/inputs/driver.go:260`
- Modify: `test/fixtures/0022-http-ext-proc-grpc/inputs/driver.go:172,182,240` (line 240 is `restartProcessorGRPC()` — the server is re-bound mid-scenario before S7/S8 and the ref container must reach the restarted instance too)
- Modify: `test/fixtures/0023-http-ext-proc-body/inputs/driver.go:171`
- Modify: `test/fixtures/0032-http-ratelimit/inputs/driver.go:277`

The pattern in every file: a server *bind* address built as `fmt.Sprintf("127.0.0.1:%d", port)`. Change the bind host to `0.0.0.0` so the ref container can reach the service through the bridge gateway. Subject-side clients dialing `127.0.0.1:<port>` keep working — a `0.0.0.0` bind accepts loopback connections.

- [ ] **Step 1: For each listed line, verify it is a server bind, not a client dial**

For each file, read ~10 lines of context around the listed line. The address must flow into a `net.Listen` / server-start helper (e.g. `jwksbackendNew(ctx, addr, ...)` at 0019:218, `setupAuthGRPC` re-bind at 0021:260, server starts at 0022:172/182, and the `extprocgrpc.NewAtAddr` re-bind inside `restartProcessorGRPC()` at 0022:240 — all seven are binds and all must change). Actual dial addresses live in the bootstrap/subject config templates, not in these allocation/start functions. Do NOT touch the `net.Listen("tcp", "127.0.0.1:0")` alloc-close port reservations (e.g. 0019:201) — they never serve traffic; only the persistent server binds matter.

- [ ] **Step 2: Apply the edit in each file**

Example (0019-http-jwt-authn/inputs/driver.go:218):

```go
// before
srv, err := jwksbackendNew(context.Background(), fmt.Sprintf("127.0.0.1:%d", port), routes)
// after — bind all interfaces so the reference Envoy container can reach the
// service via host.docker.internal (bridge gateway) on plain Linux Docker;
// loopback-only binds are unreachable from containers outside Docker Desktop.
srv, err := jwksbackendNew(context.Background(), fmt.Sprintf("0.0.0.0:%d", port), routes)
```

Apply the same `127.0.0.1:%d` → `0.0.0.0:%d` change at each verified bind site.

- [ ] **Step 3: Run the six affected subtests locally (regression guard — they already pass locally)**

```bash
go test ./test/differential/ -run 'TestDifferential/(0019|0020|0021|0022|0023|0032)' -timeout 5m -v 2>&1 | tail -20
```

Expected: all six PASS.

- [ ] **Step 4: Commit**

```bash
git add test/fixtures/0019-http-jwt-authn/inputs/driver.go \
        test/fixtures/0020-http-ext-authz-http/inputs/driver.go \
        test/fixtures/0021-http-ext-authz-grpc/inputs/driver.go \
        test/fixtures/0022-http-ext-proc-grpc/inputs/driver.go \
        test/fixtures/0023-http-ext-proc-body/inputs/driver.go \
        test/fixtures/0032-http-ratelimit/inputs/driver.go
git commit -m "test/differential: bind ref-facing aux services to 0.0.0.0 so the reference container can reach them on plain Linux Docker (CI)"
```

- [ ] **Step 5: Push and verify on CI (this is the real red→green check)**

```bash
git push -u origin fix/ci-github-actions
gh run watch "$(gh run list --branch fix/ci-github-actions --limit 1 --json databaseId --jq '.[0].databaseId')" --exit-status || true
gh run view "$(gh run list --branch fix/ci-github-actions --limit 1 --json databaseId --jq '.[0].databaseId')" --log-failed | grep -E '^\S+\s+\S+.*--- FAIL' || echo "no differential FAILs"
```

Expected: 0019/0020/0021/0022/0023/0032 no longer in the FAIL list. (0006 and possibly 0036 still fail — fixed by Tasks 3–4. The lint job may also flake — Task 5.)

---

### Task 3: Move the ref access-log out of the container's /tmp (fixes B — 0006)

REQUIRED SUB-SKILL: superpowers:systematic-debugging — this task is hypothesis-driven. The obvious mode/chmod fixes are **already in the code and proven insufficient** (see inventory row B). Do not re-add chmod logic.

**Files:**
- Modify: `test/fixtures/0006-access-log/driver/driver.go:52` (`refContainerLogPath` const) and `:620` (the same path inside the reference bootstrap template; comments at `:17` and `:598` mention it too)
- Modify: `test/fixtures/0006-access-log/envoy.yaml:24` (and the comment at `:4`) — keep the canonical fixture YAML consistent with the driver template

**Hypothesis being tested:** `fs.protected_regular=2` (Ubuntu runner default) makes Envoy's `O_CREAT` open of `/tmp/envoy-access.log` fail with EACCES because the bind-mounted file is owned by uid 1001 inside a sticky world-writable directory, and Envoy runs as uid 101. Moving the file to a non-sticky directory removes the restriction; the existing 0666 file mode then permits the write. The path change is simultaneously the minimal hypothesis test and the fix.

- [ ] **Step 1: Confirm the no-op trap** — read `test/differential/runner_test.go:963-975` and verify the pre-create + `os.Chmod(hm.HostPath, 0o666)` already exist (they do; this guards against an implementer "fixing" modes again).

- [ ] **Step 2: Change the container-side log path**

In `test/fixtures/0006-access-log/driver/driver.go`:

```go
// before
const refContainerLogPath = "/tmp/envoy-access.log"
// after — /tmp in the envoy image is sticky+world-writable, so with
// fs.protected_regular=2 (Ubuntu CI default) envoy (uid 101) gets EACCES
// opening the bind-mounted file owned by the host runner uid with O_CREAT.
// A file mounted under a non-sticky directory is exempt from that sysctl.
const refContainerLogPath = "/envoy-go-test/envoy-access.log"
```

Update the `path:` line in the reference bootstrap template at driver.go:620 to the same value, and mirror the change in `test/fixtures/0006-access-log/envoy.yaml:24`. Update the stale path mentions in the comments (driver.go:17, :598, envoy.yaml:4, and test/differential/harness.go:162). Docker creates the mount target's parent directory (root-owned, 0755, non-sticky) automatically; envoy only needs the 0666 file mode plus directory traversal.

- [ ] **Step 3: Run 0006 locally (regression guard — it passes locally both before and after)**

```bash
go test ./test/differential/ -run 'TestDifferential/0006' -timeout 5m -v 2>&1 | tail -5
```

Expected: PASS.

- [ ] **Step 4: Commit and push**

```bash
git add test/fixtures/0006-access-log/
git commit -m "test/fixtures/0006: move ref access-log mount out of /tmp; fs.protected_regular=2 on CI runners EACCESes envoy's O_CREAT open of a foreign-owned file in a sticky dir"
git push
```

- [ ] **Step 5: Verify the hypothesis on CI**

```bash
gh run watch "$(gh run list --branch fix/ci-github-actions --limit 1 --json databaseId --jq '.[0].databaseId')" --exit-status --interval 30 || true
gh run view "$(gh run list --branch fix/ci-github-actions --limit 1 --json databaseId --jq '.[0].databaseId')" --log-failed | grep "0006" || echo "0006 green"
```

Expected: `0006 green` — hypothesis confirmed. **If 0006 still fails:** the hypothesis is refuted; return to systematic-debugging Phase 1. Gather evidence with a temporary diagnostic commit that (a) prints `sysctl fs.protected_regular` in a workflow step and (b) logs `ls -ln` of the host file plus `docker exec <ctr> ls -ln /tmp` before envoy boots — then form the next single hypothesis (e.g. ENVOY_UID handling, AppArmor) from that output. Do not stack speculative fixes.

---

### Task 4: Retry subject start on transient port collision (fixes C — 0036 flake)

**Files:**
- Modify: `test/differential/runner_test.go:986-993` (the `subjPort := freeTCPPort(t)` … `StartSubjectProxy` block in the main runner)

Multi-listener fixtures use `subjPort+1..+3`; none of the derived ports are reserved, so a collision is always possible. Cheapest robust fix: retry the whole allocate→render-config→start sequence when the failure is a bind collision.

- [ ] **Step 1: Wrap subject start in a bounded retry**

Replace the block:

```go
subjPort := freeTCPPort(t)
subjAdminPort := freeTCPPort(t)
subjCfg := d.SubjectConfig(d.ReferenceListenerPort(), subjPort, backendPorts, subjAdminPort)
subj, err := StartSubjectProxy(ctx, root, subjCfg, fmt.Sprintf("127.0.0.1:%d", subjAdminPort))
if err != nil {
    t.Fatalf("subj start: %v", err)
}
```

with:

```go
// Multi-listener fixtures derive ports as subjPort+1..+N without reserving
// them, so a freshly-allocated base port can collide with a port grabbed in
// the window between freeTCPPort's close and envoy-go's bind. Retry the whole
// allocate→render→start sequence on bind collisions (seen on CI as
// "address already in use" followed by "subject ready: EOF").
var subj *SubjectProxy
var subjAdminPort int
for attempt := 1; ; attempt++ {
    subjPort := freeTCPPort(t)
    subjAdminPort = freeTCPPort(t)
    subjCfg := d.SubjectConfig(d.ReferenceListenerPort(), subjPort, backendPorts, subjAdminPort)
    var err error
    subj, err = StartSubjectProxy(ctx, root, subjCfg, fmt.Sprintf("127.0.0.1:%d", subjAdminPort))
    if err == nil {
        break
    }
    if attempt >= 3 {
        t.Fatalf("subj start (attempt %d): %v", attempt, err)
    }
    t.Logf("subj start attempt %d failed (%v); retrying with fresh ports", attempt, err)
}
```

Note: `StartSubjectProxy`'s error on a bind collision surfaces as `subject ready: EOF` (the process exits before the readiness probe), not as a string containing "address already in use" — so retry on *any* start error, bounded at 3 attempts. A real config bug still fails fast (deterministically, 3×). `subjPort`/`subjAdminPort` are not referenced after this block in the current code (admin access goes through the `subj` handle), so loop-scoping them is safe — but re-verify with a quick grep before committing.

- [ ] **Step 2: Compile and run a multi-listener fixture locally**

```bash
go vet ./test/differential/
go test ./test/differential/ -run 'TestDifferential/(0023|0036)' -timeout 5m -v 2>&1 | tail -5
```

Expected: PASS.

- [ ] **Step 3: Commit and push**

```bash
git add test/differential/runner_test.go
git commit -m "test/differential: retry subject start with fresh ports on transient bind collisions (CI 0036 flake)"
git push
```

Scope note: the same unprotected allocate→start pattern exists in the other runner functions at runner_test.go:1738 and :1880. CI evidence implicates only the main runner (the fatal message matches line 991), so this task deliberately patches only that site — if the same flake shape appears at the other sites during the Task 7 soak, extend the retry there too.

---

### Task 5: Investigate and deflake the -short unit tests (D)

**Files:**
- Investigate: `internal/filter/http/wasm/dispatch_test.go` (`TestFilter_RootVM_SharedAcrossStreams_NoCrossStreamLeak`, `TestFilter_ConcurrentStreams_NoSharedState`)
- Investigate: `internal/sdsfile/sdsfile_test.go` (`TestWatcher_Start_ObservesAtomicRename`)

REQUIRED SUB-SKILL: superpowers:systematic-debugging. These are intermittent; **do not patch a test without first reproducing its failure and identifying the root cause.** A flake in the wasm dispatch tests may be a genuine concurrency bug in the dispatch path (shared root VM called from multiple streams) — that distinction decides whether the fix lands in test code or production code.

- [ ] **Step 1: Reproduce under CI-like constraints**

```bash
GOMAXPROCS=2 go test -short -race -count=200 -failfast ./internal/filter/http/wasm/ 2>&1 | tail -20
GOMAXPROCS=2 go test -short -race -count=200 -failfast ./internal/sdsfile/ 2>&1 | tail -20
```

Expected: at least one reproduced failure per flaky test (the CI evidence shows each fails ~weekly). If 200 iterations stay green, escalate constraints: `-cpu 1,2` and `nice -n 19` to starve the scheduler. If still not reproducible on the (faster) dev machine, fall back to CI as the evidence source: add a temporary branch commit that runs the stress loop as a workflow step and collect its output — consistent with this plan's "CI is the real verifier" framing.

- [ ] **Step 2: Root-cause each reproduced failure**

For each: read the failing assertion, trace where the timing assumption or shared state lives, and write down the root cause before changing anything. If `-race` reports a data race in non-test code, STOP and report — that is a production bug requiring its own TDD fix, not a test tweak.

- [ ] **Step 3: Fix at the root cause** (test synchronization, condition-based waiting instead of sleeps — see systematic-debugging's `condition-based-waiting.md`)

- [ ] **Step 4: Prove the fix by re-running the Step 1 stress loops**

Expected: 200/200 green for the affected packages.

- [ ] **Step 5: Commit and push**

```bash
git add internal/filter/http/wasm/ internal/sdsfile/
git commit -m "test: deflake -short unit tests under 2-core CI runners (root causes: <fill in from investigation>)"
git push
```

---

### Task 6: Upgrade workflow actions off Node 20 (E — deadline 2026-06-16)

**Files:**
- Modify: `.github/workflows/ci.yml:11,12,21,31,32,47,48` (the three `actions/checkout@v4`, three `actions/setup-go@v5`, one `golangci/golangci-lint-action@v6`)

- [ ] **Step 1: Check current major versions**

```bash
gh api repos/actions/checkout/releases/latest --jq .tag_name
gh api repos/actions/setup-go/releases/latest --jq .tag_name
gh api repos/golangci/golangci-lint-action/releases/latest --jq .tag_name
```

- [ ] **Step 2: Bump checkout and setup-go to the latest majors** (expected `@v5` / `@v6` — Node 24 runtimes), in all three jobs.

- [ ] **Step 3: Handle golangci-lint-action carefully**

golangci-lint-action v7+ requires golangci-lint v2; this repo pins `version: v1.64.8` (ci.yml:23). Do **not** jump majors blindly. Pin the newest v6.x tag. If no v6.x supports Node 24, leave it at v6 and note in the commit message that the lint-action major bump (with the golangci-lint v1→v2 config migration) is a separate follow-up — the June 16 forced-Node-24 change affects when GitHub flips the default runtime, and the action may still run; a broken lint step would show up in CI immediately.

- [ ] **Step 4: Commit, push, verify all jobs still run**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: move actions off deprecated Node 20 runtimes ahead of 2026-06-16 forced default"
git push
gh run watch "$(gh run list --branch fix/ci-github-actions --limit 1 --json databaseId --jq '.[0].databaseId')" --exit-status || true
```

Expected: all three jobs execute (no action-resolution errors); Node 20 deprecation annotations gone for the bumped actions.

---

### Task 7: Full green verification and flake soak

REQUIRED SUB-SKILL: superpowers:verification-before-completion — no success claims without command output.

- [ ] **Step 1: Confirm the branch run is fully green**

```bash
gh run list --branch fix/ci-github-actions --limit 1
```

Expected: `completed success`.

- [ ] **Step 2: Soak for flakes — re-run the full workflow twice more**

```bash
gh run rerun "$(gh run list --branch fix/ci-github-actions --limit 1 --json databaseId --jq '.[0].databaseId')"
gh run watch ... # repeat twice
```

Expected: 3 consecutive green runs (catches C and D regressions, and the 0017-bandwidth-limit watch item). If 0017-http-bandwidth-limit fails in any soak run, open a systematic-debugging investigation for it before merging — do not merge with a known recurring flake.

- [ ] **Step 3: Merge to master** (this repo merges directly to master; follow superpowers:finishing-a-development-branch)

```bash
git checkout master && git merge --ff-only fix/ci-github-actions && git push
```

- [ ] **Step 4: Confirm the master run is green**

```bash
gh run watch "$(gh run list --branch master --limit 1 --json databaseId --jq '.[0].databaseId')" --exit-status
```

Expected: success — first green master run in ~330 runs.
